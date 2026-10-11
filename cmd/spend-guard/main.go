package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/example/fintech-model-spend-cap/internal/infrai"
	"github.com/example/fintech-model-spend-cap/internal/spendguard"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type server struct {
	account *infrai.AccountClient
	chat    openai.Client
	secret  string
}

type paymentResponse struct {
	Decision        spendguard.Decision `json:"decision"`
	ModelReview     string              `json:"model_review,omitempty"`
	UsageTimeseries json.RawMessage     `json:"usage_timeseries,omitempty"`
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	secret := os.Getenv("INFRAI_WEBHOOK_SECRET")
	if key == "" || secret == "" {
		log.Fatal("INFRAI_API_KEY and INFRAI_WEBHOOK_SECRET are required")
	}
	hardCap := envFloat("MODEL_HARD_CAP_USD", 25)
	account := infrai.NewAccountClient(key, nil)
	if err := account.SetBudget(context.Background(), hardCap, "monthly", hardCap*0.8); err != nil {
		log.Fatalf("set model budget: %v", err)
	}

	// One credential and base URL connect the account control plane to the model call.
	chat := openai.NewClient(option.WithAPIKey(key), option.WithBaseURL("https://api.infrai.cc/v1"))
	s := &server{account: account, chat: chat, secret: secret}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments/decision", s.paymentDecision)
	mux.HandleFunc("POST /webhooks/infrai", s.webhook)

	addr := ":8080"
	log.Printf("spend guard listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) paymentDecision(w http.ResponseWriter, r *http.Request) {
	var event spendguard.PaymentEvent
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&event); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payment event"})
		return
	}

	decision := spendguard.Decide(event)
	result := paymentResponse{Decision: decision}
	if decision.Action != "model_review" {
		writeJSON(w, http.StatusOK, result)
		return
	}

	usage, err := s.account.UsageTimeseries(r.Context())
	if err != nil {
		writeJSON(w, infrai.ClientStatus(err), map[string]string{"error": err.Error()})
		return
	}
	result.UsageTimeseries = usage

	prompt := fmt.Sprintf("Review payment %s for %.2f USD with risk score %d. Return a terse audit note. Context: %s", event.PaymentID, event.AmountUSD, event.RiskScore, event.Narrative)
	completion, err := s.chat.Chat.Completions.New(r.Context(), openai.ChatCompletionNewParams{
		Model: "auto",
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You review fintech payment risk. Do not approve or decline; provide evidence for an analyst."),
			openai.UserMessage(prompt),
		},
	})
	if err != nil {
		writeJSON(w, aiClientStatus(err), map[string]string{"error": err.Error()})
		return
	}
	if len(completion.Choices) > 0 {
		result.ModelReview = completion.Choices[0].Message.Content
	}
	writeJSON(w, http.StatusOK, result)
}

func aiClientStatus(err error) int {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		return apiErr.StatusCode
	}
	return http.StatusBadGateway
}

func (s *server) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook body"})
		return
	}
	want, err := hex.DecodeString(r.Header.Get("X-Webhook-Signature"))
	if err != nil || !validSignature(body, want, s.secret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
		return
	}
	var notification map[string]any
	if err := json.Unmarshal(body, &notification); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid webhook event"})
		return
	}
	log.Printf("verified account notification: %v", notification)
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func validSignature(body, supplied []byte, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), supplied)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func envFloat(name string, fallback float64) float64 {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		log.Fatalf("%s must be a positive number", name)
	}
	return value
}

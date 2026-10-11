package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookVerifiesSignature(t *testing.T) {
	body := `{"event":"budget.threshold","amount_usd":20}`
	secret := "test-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	signature := hex.EncodeToString(mac.Sum(nil))

	s := &server{secret: secret}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/infrai", strings.NewReader(body))
	req.Header.Set("X-Webhook-Signature", signature)
	res := httptest.NewRecorder()
	s.webhook(res, req)

	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
}

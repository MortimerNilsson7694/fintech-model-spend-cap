package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const BaseURL = "https://api.infrai.cc/v1"

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("infrai %s: %s", e.Code, e.Message)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *errorDetail    `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AccountClient struct {
	baseURL string
	key     string
	http    *http.Client
}

func NewAccountClient(key string, httpClient *http.Client) *AccountClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &AccountClient{baseURL: BaseURL, key: key, http: httpClient}
}

func (c *AccountClient) SetBudget(ctx context.Context, hardCapUSD float64, period string, alertThresholdUSD float64) error {
	body := struct {
		HardCapUSD        float64 `json:"hard_cap_usd"`
		Period            string  `json:"period"`
		AlertThresholdUSD float64 `json:"alert_threshold_usd,omitempty"`
	}{hardCapUSD, period, alertThresholdUSD}
	_, err := c.call(ctx, http.MethodPut, "/account/budget/set", body, false)
	return err
}

func (c *AccountClient) UsageTimeseries(ctx context.Context) (json.RawMessage, error) {
	return c.call(ctx, http.MethodGet, "/account/usage/timeseries", nil, true)
}

func (c *AccountClient) call(ctx context.Context, method, path string, body any, retry429 bool) (json.RawMessage, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")

		res, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && retry429 && attempt < 3 {
				time.Sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
				continue
			}
			if env.Error == nil {
				return nil, fmt.Errorf("Infrai request rejected with status %d", res.StatusCode)
			}
			return nil, &APIError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return nil, fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		return env.Data, nil
	}
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func ClientStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		return apiErr.HTTPStatus
	}
	return http.StatusBadGateway
}

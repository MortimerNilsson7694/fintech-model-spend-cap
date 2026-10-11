package spendguard

import "fmt"

type PaymentEvent struct {
	PaymentID string  `json:"payment_id"`
	AmountUSD float64 `json:"amount_usd"`
	RiskScore int     `json:"risk_score"`
	Narrative string  `json:"narrative"`
}

type Decision struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func Decide(event PaymentEvent) Decision {
	switch {
	case event.PaymentID == "" || event.AmountUSD <= 0:
		return Decision{Action: "reject", Reason: "invalid payment event"}
	case event.RiskScore >= 80:
		return Decision{Action: "manual_review", Reason: "high risk score"}
	case event.AmountUSD >= 10_000 || event.RiskScore >= 50:
		return Decision{Action: "model_review", Reason: "model review required"}
	default:
		return Decision{Action: "approve", Reason: fmt.Sprintf("routine payment %.2f USD", event.AmountUSD)}
	}
}

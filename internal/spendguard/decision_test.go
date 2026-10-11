package spendguard

import "testing"

func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		in   PaymentEvent
		want string
	}{
		{"routine payment", PaymentEvent{PaymentID: "pay_101", AmountUSD: 42, RiskScore: 12}, "approve"},
		{"large payment", PaymentEvent{PaymentID: "pay_102", AmountUSD: 12_500, RiskScore: 20}, "model_review"},
		{"elevated risk", PaymentEvent{PaymentID: "pay_103", AmountUSD: 80, RiskScore: 61}, "model_review"},
		{"high risk", PaymentEvent{PaymentID: "pay_104", AmountUSD: 80, RiskScore: 88}, "manual_review"},
		{"missing id", PaymentEvent{AmountUSD: 80, RiskScore: 10}, "reject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide(tt.in); got.Action != tt.want {
				t.Fatalf("Decide() action = %q, want %q", got.Action, tt.want)
			}
		})
	}
}

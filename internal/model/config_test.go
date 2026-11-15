package model_test

import (
	"testing"

	"token-ray/internal/model"
)

func TestBillingModeValidation(t *testing.T) {
	tests := []struct {
		input string
		want  model.BillingMode
		valid bool
	}{
		{"auto", model.BillingAuto, true},
		{"subscription", model.BillingSubscription, true},
		{"api", model.BillingAPI, true},
		{"unknown", "", false},
		{"", model.BillingAuto, true}, // empty defaults to auto
	}

	for _, tt := range tests {
		mode, err := model.ParseBillingMode(tt.input)
		if tt.valid && err != nil {
			t.Errorf("ParseBillingMode(%q): unexpected error %v", tt.input, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("ParseBillingMode(%q): expected error, got %v", tt.input, mode)
		}
		if tt.valid && mode != tt.want {
			t.Errorf("ParseBillingMode(%q) = %q, want %q", tt.input, mode, tt.want)
		}
	}
}

func TestPlanLimits(t *testing.T) {
	plans := []string{"pro", "max5", "max20"}
	for _, p := range plans {
		limit, ok := model.PlanLimits[p]
		if !ok {
			t.Errorf("PlanLimits missing plan %q", p)
			continue
		}
		if limit.TokenLimit <= 0 {
			t.Errorf("plan %q: token limit should be > 0", p)
		}
		if limit.CostLimit <= 0 {
			t.Errorf("plan %q: cost limit should be > 0", p)
		}
	}
}

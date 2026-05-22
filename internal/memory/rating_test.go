package memory

import "testing"

func TestParseRating(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Explicit rating label buy", "Rating: Buy", "Buy"},
		{"Explicit rating label markdown overweight", "some text\nRating: **Overweight**\nother text", "Overweight"},
		{"Explicit rating label hypen sell", "Rating - Sell", "Sell"},
		{"Word list fallback underweight", "This is an underweight position.", "Underweight"},
		{"Multiple rating words first matches", "First we hold then we buy.", "Hold"},
		{"No rating words default hold", "This is some neutral analysis text.", "Hold"},
		{"Custom default", "No rating here.", "Sell"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.name == "Custom default" {
				got = ParseRating(tc.input, "Sell")
			} else {
				got = ParseRating(tc.input)
			}
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

package memory

import (
	"regexp"
	"strings"
)

// Ratings5Tier represents the canonical, ordered 5-tier scale.
var Ratings5Tier = []string{"Buy", "Overweight", "Hold", "Underweight", "Sell"}

var ratingsSet = map[string]string{
	"buy":         "Buy",
	"overweight":  "Overweight",
	"hold":        "Hold",
	"underweight": "Underweight",
	"sell":        "Sell",
}

// ratingLabelRe matches "Rating: X", "rating - X", "Rating: **X**", tolerating markdown wrappers.
var ratingLabelRe = regexp.MustCompile(`(?i)rating.*?[:\-][\s*]*(\w+)`)

// ParseRating heuristically extracts a 5-tier rating from prose text.
//
// Two-pass strategy:
// 1. Look for an explicit "Rating: X" label (tolerant of markdown bold).
// 2. Fall back to the first 5-tier rating word found anywhere in the text.
//
// Returns a Title-cased rating string, or default "Hold" if no rating word appears.
func ParseRating(text string, defaultVal ...string) string {
	dVal := "Hold"
	if len(defaultVal) > 0 {
		dVal = defaultVal[0]
	}

	lines := strings.Split(text, "\n")

	// Pass 1: Look for explicit "Rating: X" pattern
	for _, line := range lines {
		m := ratingLabelRe.FindStringSubmatch(line)
		if len(m) > 1 {
			candidate := strings.ToLower(m[1])
			if canonical, exists := ratingsSet[candidate]; exists {
				return canonical
			}
		}
	}

	// Pass 2: Search for the first rating word in text
	for _, line := range lines {
		words := strings.Fields(line)
		for _, word := range words {
			// Clean markdown bold or punctuation wrappers: "*:.,"
			clean := strings.Trim(strings.ToLower(word), "*:.,")
			if canonical, exists := ratingsSet[clean]; exists {
				return canonical
			}
		}
	}

	return dVal
}

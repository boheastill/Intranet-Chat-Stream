package ai

import "strings"

// route maps a message trigger token to a target backend name.
type route struct {
	token   string
	backend string
}

// triggers is the trigger→backend routing table — the single source of truth
// shared by the pipeline (which wakes on any of these tokens) and the template
// backend (which routes by them).
//
//   - @ds → DeepSeek
//   - @mi → MiMo
//   - @ag → placeholder (no agent backend yet; falls back to DeepSeek)
//   - @cc → legacy trigger, now an alias that defaults to MiMo
//
// Tokens are distinct (none is a substring of another), so when several appear
// in one message the one nearest the start of the text wins (see MatchTrigger).
var triggers = []route{
	{"@ds", "deepseek"},
	{"@mi", "mimo"},
	{"@ag", "deepseek"},
	{"@cc", "mimo"},
}

// TriggerTokens returns all recognized trigger tokens (for logging / wake-up).
func TriggerTokens() []string {
	tokens := make([]string, len(triggers))
	for i, r := range triggers {
		tokens[i] = r.token
	}
	return tokens
}

// MatchTrigger returns the token and backend for the trigger that appears
// earliest in content, and whether any trigger was present. Matching is
// case-insensitive.
func MatchTrigger(content string) (token, backend string, ok bool) {
	lower := strings.ToLower(content)
	bestPos := -1
	for _, r := range triggers {
		if i := strings.Index(lower, r.token); i >= 0 && (bestPos == -1 || i < bestPos) {
			bestPos, token, backend, ok = i, r.token, r.backend, true
		}
	}
	return token, backend, ok
}

// stripTrigger removes the earliest trigger token from content and returns the
// remaining text, trimmed. If no trigger is present the trimmed content is returned.
func stripTrigger(content string) string {
	lower := strings.ToLower(content)
	bestPos, bestLen := -1, 0
	for _, r := range triggers {
		if i := strings.Index(lower, r.token); i >= 0 && (bestPos == -1 || i < bestPos) {
			bestPos, bestLen = i, len(r.token)
		}
	}
	if bestPos == -1 {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(content[:bestPos] + content[bestPos+bestLen:])
}

package ai

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TemplateBackend is a zero-cost pattern-matching backend.
// Handles simple messages without calling any AI API.
type TemplateBackend struct{}

func NewTemplate() *TemplateBackend { return &TemplateBackend{} }

func (t *TemplateBackend) Name() string { return "template" }

func (t *TemplateBackend) Process(ctx context.Context, msg Message, knowledge []Entry) (*Response, error) {
	content := strings.TrimSpace(msg.Content)

	switch {
	case containsAny(content, "test"):
		return &Response{Text: "Test passed."}, nil

	case containsAny(content, "hi", "hello"):
		return &Response{Text: "Hello! What do you need?"}, nil

	case containsAny(content, "status"):
		return &Response{Text: fmt.Sprintf("ICS-Pipeline running | %s", time.Now().Format("15:04:05"))}, nil

	case containsAny(content, "time"):
		return &Response{Text: fmt.Sprintf("%s", time.Now().Format("2006-01-02 15:04:05"))}, nil

	default:
		return &Response{
			Text:   "",
			Action: "delegate:deepseek",
		}, nil
	}
}

func containsAny(s string, keywords ...string) bool {
	lower := strings.ToLower(s)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

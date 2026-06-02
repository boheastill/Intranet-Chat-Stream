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
	rest := strings.TrimSpace(stripTrigger(msg.Content))

	// A bare trigger with no question (e.g. just "@ag") would otherwise call an
	// AI backend with empty input — reply with usage instead of wasting a call.
	if rest == "" {
		return &Response{Text: "用法：@ds 你的问题(DeepSeek) · @mi(小米 MiMo) · @cc/@ag。触发词后写上要问的内容。"}, nil
	}

	// Instant commands match the message with its trigger token stripped, so a
	// plain "@cc test" replies instantly while "@ds explain time complexity"
	// (which merely contains "time") is routed to a real backend instead.
	switch strings.ToLower(rest) {
	case "test":
		return &Response{Text: "Test passed."}, nil
	case "hi", "hello":
		return &Response{Text: "Hello! What do you need?"}, nil
	case "status":
		return &Response{Text: fmt.Sprintf("ICS-Pipeline running | %s", time.Now().Format("15:04:05"))}, nil
	case "time":
		return &Response{Text: time.Now().Format("2006-01-02 15:04:05")}, nil
	}

	// Route to the backend mapped by the trigger token present in the message.
	if _, backend, ok := MatchTrigger(msg.Content); ok {
		return &Response{Action: "delegate:" + backend}, nil
	}

	// Defensive fallback: the pipeline only invokes us when a trigger is present.
	return &Response{Action: "delegate:deepseek"}, nil
}

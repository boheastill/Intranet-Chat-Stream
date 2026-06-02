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
	// Instant commands match the message with its trigger token stripped, so a
	// plain "@cc test" replies instantly while "@ds explain time complexity"
	// (which merely contains "time") is routed to a real backend instead.
	switch strings.ToLower(stripTrigger(msg.Content)) {
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

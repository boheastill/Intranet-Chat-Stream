// Package ai defines the universal AI backend interface.
// To add a new AI: implement this interface, register it in main.
package ai

import "context"

// Message represents an incoming user message to be processed.
type Message struct {
	ID      string
	Content string
	Device  string
	Channel string
	Trigger string
}

// Response represents the AI's reply.
type Response struct {
	Text   string
	Action string // "" or "delegate:backend_name"
}

// Entry is a knowledge base entry.
type Entry struct {
	ID      string   `json:"id"`
	Topic   string   `json:"topic"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
	Time    string   `json:"time"`
}

// Backend is the universal AI interface.
// Every AI implements this: Template, DeepSeek, Claude, etc.
type Backend interface {
	Name() string
	Process(ctx context.Context, msg Message, knowledge []Entry) (*Response, error)
}

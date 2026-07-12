package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// MiMoBackend calls Xiaomi MiMo API (Anthropic-compatible).
type MiMoBackend struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewMiMo() *MiMoBackend {
	key := os.Getenv("MIMO_API_KEY")
	model := os.Getenv("MIMO_MODEL")
	if model == "" {
		model = "mimo-v2.5-pro"
	}
	return &MiMoBackend{
		apiKey:  key,
		baseURL: "https://api.xiaomimimo.com/anthropic",
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (m *MiMoBackend) Name() string { return "mimo" }

func (m *MiMoBackend) Process(ctx context.Context, msg Message, knowledge []Entry) (*Response, error) {
	if m.apiKey == "" {
		return nil, fmt.Errorf("mimo: MIMO_API_KEY not set")
	}

	systemPrompt := buildMiMoSystemPrompt(knowledge)

	// Anthropic Messages API format
	body := map[string]any{
		"model":      m.model,
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": fmt.Sprintf("[from %s] %s", msg.Device, msg.Content)},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", m.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", m.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mimo api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("mimo api %d: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mimo decode: %w", err)
	}

	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			return &Response{Text: c.Text}, nil
		}
	}

	return &Response{Text: "(empty response)"}, nil
}

func buildMiMoSystemPrompt(knowledge []Entry) string {
	var sb strings.Builder
	sb.WriteString("You are the user's AI assistant running in ICS-Pipeline.\n")
	sb.WriteString("Your brain is Xiaomi MiMo (mimo-v2.5-pro), invoked by a Go binary.\n")
	sb.WriteString("Reply concisely, accurately, max 3 sentences.\n")
	sb.WriteString("Current time: " + time.Now().Format("2006-01-02 15:04:05") + "\n")

	if len(knowledge) > 0 {
		sb.WriteString("\n--- Authoritative facts about ICS (ground truth; do not contradict) ---\n")
		for _, e := range knowledge {
			fmt.Fprintf(&sb, "[%s] %s\n", e.Topic, e.Content)
		}
	}

	return sb.String()
}

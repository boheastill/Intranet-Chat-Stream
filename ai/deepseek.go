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

// DeepSeekBackend calls DeepSeek API (openai-compatible).
type DeepSeekBackend struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewDeepSeek() *DeepSeekBackend {
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		key = "sk-1be4ea85114c4b28b3ada0b8a660a9f9"
	}
	return &DeepSeekBackend{
		apiKey:  key,
		baseURL: "https://api.deepseek.com/v1",
		model:   "deepseek-chat",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (d *DeepSeekBackend) Name() string { return "deepseek" }

func (d *DeepSeekBackend) Process(ctx context.Context, msg Message, knowledge []Entry) (*Response, error) {
	systemPrompt := buildSystemPrompt(knowledge)

	body := map[string]interface{}{
		"model": d.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("[from %s] %s", msg.Device, msg.Content)},
		},
		"max_tokens":  1024,
		"temperature": 0.7,
	}

	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", d.baseURL+"/chat/completions", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek api: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("deepseek decode: %w", err)
	}

	if len(result.Choices) == 0 {
		return &Response{Text: "(empty response)"}, nil
	}

	return &Response{Text: result.Choices[0].Message.Content}, nil
}

func buildSystemPrompt(knowledge []Entry) string {
	var sb strings.Builder
	sb.WriteString("You are bohea's AI assistant running in ICS-Pipeline Mode C.\n")
	sb.WriteString("Your brain is DeepSeek API (deepseek-chat), invoked by a Go binary.\n")
	sb.WriteString("You are ICS-Pipeline DeepSeek backend, invoked by a Go binary.\n")
	sb.WriteString("Reply concisely, accurately, max 3 sentences.\n")
	sb.WriteString("Current time: " + time.Now().Format("2006-01-02 15:04:05") + "\n")

	if len(knowledge) > 0 {
		sb.WriteString("\n--- Context ---\n")
		for _, e := range knowledge {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", e.Topic, e.Content))
		}
	}

	return sb.String()
}

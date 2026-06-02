// Package pipeline is the SSE consumer, AI router, and knowledge base.
// It runs as a goroutine alongside the Message Bus: it listens for new
// messages, and when one carries a trigger token (see ai.Triggers — @ds, @mi,
// @ag, @cc) it routes to the mapped AI backend and pushes the reply back.
package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"ics/ai"
	"ics/knowledge"
)

const (
	busBaseURL     = "https://flow.bohea.us"
	busStreamURL   = busBaseURL + "/api/stream"
	busPushURL     = busBaseURL + "/api/push"
	busDownloadURL = busBaseURL + "/api/download/"
	seenStatePath  = "pipeline.seen.json"
	knowledgePath  = "pipeline.knowledge.json"
)

var (
	authToken string
	backends  map[string]ai.Backend
	kb        *knowledge.Store
	seen      = make(map[string]bool)
)

// Start runs the pipeline loop forever, authenticating to the bus with token.
func Start(token string) {
	authToken = token

	var err error
	kb, err = knowledge.New(knowledgePath)
	if err != nil {
		log.Printf("[pipeline] knowledge init: %v", err)
	}

	backends = map[string]ai.Backend{
		"template": ai.NewTemplate(),
		"deepseek": ai.NewDeepSeek(),
		"mimo":     ai.NewMiMo(),
	}

	loadSeen()

	log.Printf("[pipeline] Backends: template, deepseek, mimo")
	log.Printf("[pipeline] SSE: %s  Triggers: %s", busStreamURL, strings.Join(ai.TriggerTokens(), " "))

	for {
		if err := listenSSE(); err != nil {
			log.Printf("[pipeline] SSE: %v — reconnecting in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}
}

func listenSSE() error {
	req, _ := http.NewRequest("GET", busStreamURL, nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Auth-Token", authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	log.Printf("[pipeline] Connected to SSE")
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var evt struct {
			Event   string `json:"event"`
			Channel string `json:"channel"`
			ID      string `json:"id"`
		}
		data := strings.TrimPrefix(line, "data: ")
		if json.Unmarshal([]byte(data), &evt) != nil {
			continue
		}
		if evt.Event != "new_msg" || evt.ID == "" {
			continue
		}
		if strings.Contains(evt.ID, "_ai_") || alreadySeen(evt.ID) {
			continue
		}
		markSeen(evt.ID)
		handleMessage(evt.ID, evt.Channel)
	}
	return scanner.Err()
}

func handleMessage(id, channel string) {
	content := downloadMessage(id)
	token, _, ok := ai.MatchTrigger(content)
	if content == "" || !ok {
		return
	}
	device := extractDevice(id)
	log.Printf("[pipeline] %s from %s", token, device)

	msg := ai.Message{
		ID:      id,
		Content: content,
		Device:  device,
		Channel: channel,
		Trigger: token,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	relevant := kb.Search(msg.Content, 5)

	// Try template
	resp, err := backends["template"].Process(ctx, msg, relevant)
	if err == nil && resp != nil && resp.Text != "" {
		log.Printf("[pipeline] template: %s", truncate(resp.Text, 60))
		pushReply(resp.Text, "ai")
		kb.RecordConversation("last_reply", truncate(resp.Text, 200))
		return
	}

	// Delegate
	if resp != nil && strings.HasPrefix(resp.Action, "delegate:") {
		target := strings.TrimPrefix(resp.Action, "delegate:")
		if backend, ok := backends[target]; ok {
			log.Printf("[pipeline] → %s", target)
			aiResp, err := backend.Process(ctx, msg, relevant)
			if err != nil {
				log.Printf("[pipeline] %s: %v", target, err)
				pushReply("AI unavailable, try later.", "ai")
				return
			}
			if aiResp.Text != "" {
				log.Printf("[pipeline] %s: %s", target, truncate(aiResp.Text, 60))
				pushReply(aiResp.Text, "ai")
				kb.RecordConversation("last_reply", truncate(aiResp.Text, 200))
				return
			}
		}
	}

	pushReply("Message received. #"+msg.ID, "ai")
}

func downloadMessage(id string) string {
	resp, err := http.Get(busDownloadURL + id + "?token=" + authToken)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body := make([]byte, 32*1024)
	n, _ := resp.Body.Read(body)
	return string(body[:n])
}

func pushReply(text, device string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("text", text)
	_ = w.WriteField("device", device)
	_ = w.Close()

	req, _ := http.NewRequest("POST", busPushURL, &buf)
	req.Header.Set("X-Auth-Token", authToken)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[pipeline] push: %v", err)
		return
	}
	_ = resp.Body.Close()
}

func extractDevice(id string) string {
	parts := strings.Split(id, "_")
	if len(parts) >= 2 {
		d := parts[1]
		if d == "pc" || d == "mobile" || d == "ai" || d == "web" {
			return d
		}
	}
	return "unknown"
}

func alreadySeen(id string) bool {
	if seen[id] {
		return true
	}
	if len(seen) > 1000 {
		seen = make(map[string]bool)
	}
	return false
}

func markSeen(id string) {
	seen[id] = true
}

func loadSeen() {
	data, err := os.ReadFile(seenStatePath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &seen)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

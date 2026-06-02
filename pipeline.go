package main

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

	"clipstream/ai"
	"clipstream/knowledge"
)

// Pipeline — SSE consumer, AI router, knowledge base.
// Runs as a goroutine alongside the Message Bus HTTP server.

const (
	icsBase     = "https://flow.bohea.us"
	ppsStream   = icsBase + "/api/stream"
	ppsList     = icsBase + "/api/list"
	ppsPush     = icsBase + "/api/push"
	ppsDownload = icsBase + "/api/download/"
	ppsTrigger  = "@cc"
	ppsSeenFile = "pipeline_seen.json"
	ppsKBPath   = "pipeline_knowledge.json"
)

var (
	ppBackends map[string]ai.Backend
	ppKB       *knowledge.Store
	ppSeen     = make(map[string]bool)
)

func startPipeline() {
	var err error
	ppKB, err = knowledge.New(ppsKBPath)
	if err != nil {
		log.Printf("[pipeline] knowledge init: %v", err)
	}

	ppBackends = map[string]ai.Backend{
		"template": ai.NewTemplate(),
		"deepseek": ai.NewDeepSeek(),
	}

	ppLoadSeen()

	log.Printf("[pipeline] Backends: template, deepseek")
	log.Printf("[pipeline] SSE: %s  Trigger: %s", ppsStream, ppsTrigger)

	for {
		err := ppListenSSE()
		if err != nil {
			log.Printf("[pipeline] SSE: %v — reconnecting in 5s", err)
			time.Sleep(5 * time.Second)
		}
	}
}

func ppListenSSE() error {
	req, _ := http.NewRequest("GET", ppsStream, nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Auth-Token", globalConfig.Token)

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
		if strings.Contains(evt.ID, "_ai_") || ppAlreadySeen(evt.ID) {
			continue
		}
		ppMarkSeen(evt.ID)
		ppHandle(evt.ID, evt.Channel)
	}
	return scanner.Err()
}

func ppHandle(id, channel string) {
	content := ppDownload(id)
	if content == "" || !strings.Contains(content, ppsTrigger) {
		return
	}
	device := extractDevice(id)
	log.Printf("[pipeline] @cc from %s", device)

	msg := ai.Message{
		ID:      id,
		Content: content,
		Device:  device,
		Channel: channel,
		Trigger: ppsTrigger,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	relevant := ppKB.Search(msg.Content, 5)

	// Try template
	resp, err := ppBackends["template"].Process(ctx, msg, relevant)
	if err == nil && resp != nil && resp.Text != "" {
		log.Printf("[pipeline] template: %s", truncate(resp.Text, 60))
		ppPush(resp.Text, "ai")
		ppKB.RecordConversation("last_reply", truncate(resp.Text, 200))
		return
	}

	// Delegate
	if resp != nil && strings.HasPrefix(resp.Action, "delegate:") {
		target := strings.TrimPrefix(resp.Action, "delegate:")
		if backend, ok := ppBackends[target]; ok {
			log.Printf("[pipeline] → %s", target)
			aiResp, err := backend.Process(ctx, msg, relevant)
			if err != nil {
				log.Printf("[pipeline] %s: %v", target, err)
				ppPush("AI unavailable, try later.", "ai")
				return
			}
			if aiResp.Text != "" {
				log.Printf("[pipeline] %s: %s", target, truncate(aiResp.Text, 60))
				ppPush(aiResp.Text, "ai")
				ppKB.RecordConversation("last_reply", truncate(aiResp.Text, 200))
				return
			}
		}
	}

	ppPush("Message received. #"+msg.ID, "ai")
}

func ppDownload(id string) string {
	resp, err := http.Get(ppsDownload + id + "?token=" + globalConfig.Token)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body := make([]byte, 32*1024)
	n, _ := resp.Body.Read(body)
	return string(body[:n])
}

func ppPush(text, device string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("text", text)
	w.WriteField("device", device)
	w.Close()

	req, _ := http.NewRequest("POST", ppsPush, &buf)
	req.Header.Set("X-Auth-Token", globalConfig.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[pipeline] push: %v", err)
		return
	}
	resp.Body.Close()
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

func ppAlreadySeen(id string) bool {
	if ppSeen[id] {
		return true
	}
	if len(ppSeen) > 1000 {
		ppSeen = make(map[string]bool)
	}
	return false
}

func ppMarkSeen(id string) {
	ppSeen[id] = true
}

func ppLoadSeen() {
	data, err := os.ReadFile(ppsSeenFile)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ppSeen)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

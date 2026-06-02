package knowledge

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"clipstream/ai"
)

// Store is a simple file-based knowledge base.
// Entries persist across Pipeline restarts.
type Store struct {
	mu      sync.RWMutex
	entries []ai.Entry
	path    string
}

func New(path string) (*Store, error) {
	s := &Store{path: path}
	if err := s.load(); err != nil {
		s.entries = []ai.Entry{}
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(data, &s.entries)
}

func (s *Store) save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Put adds or updates a knowledge entry.
func (s *Store) Put(topic, content string, tags []string) error {
	s.mu.Lock()
	for i, e := range s.entries {
		if e.Topic == topic {
			s.entries[i].Content = content
			s.entries[i].Tags = tags
			s.entries[i].Time = time.Now().Format("2006-01-02 15:04")
			s.mu.Unlock()
			return s.save()
		}
	}
	s.entries = append(s.entries, ai.Entry{
		ID:      topic,
		Topic:   topic,
		Content: content,
		Tags:    tags,
		Time:    time.Now().Format("2006-01-02 15:04"),
	})
	s.mu.Unlock()
	return s.save()
}

// Search returns entries matching the query.
func (s *Store) Search(query string, maxResults int) []ai.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lower := strings.ToLower(query)
	var results []ai.Entry

	for _, e := range s.entries {
		score := 0
		if strings.Contains(strings.ToLower(e.Topic), lower) {
			score += 10
		}
		for _, tag := range e.Tags {
			if strings.Contains(strings.ToLower(tag), lower) {
				score += 5
			}
		}
		if strings.Contains(strings.ToLower(e.Content), lower) {
			score += 1
		}
		if score > 0 {
			results = append(results, e)
		}
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results
}

// All returns all entries.
func (s *Store) All() []ai.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ai.Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// RecordConversation stores a conversation summary.
func (s *Store) RecordConversation(topic, summary string) {
	s.Put(topic, summary, []string{"conversation", "summary"})
}

package knowledge

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"ics/ai"
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
	s.seedProject()
	return s, nil
}

// seedProject upserts the built-in project knowledge (see seed.go) so the code
// stays the source of truth for those topics, while preserving any other entries.
// It also drops the legacy "last_reply" entry left by the now-removed
// RecordConversation, which used to pollute AI prompts with the previous reply.
func (s *Store) seedProject() {
	s.mu.Lock()
	kept := s.entries[:0]
	for _, e := range s.entries {
		if e.Topic != "last_reply" {
			kept = append(kept, e)
		}
	}
	s.entries = kept
	for _, seed := range projectKnowledge {
		entry := ai.Entry{
			ID:      seed.Topic,
			Topic:   seed.Topic,
			Content: seed.Content,
			Tags:    seed.Tags,
			Time:    "builtin",
		}
		idx := -1
		for i, e := range s.entries {
			if e.Topic == seed.Topic {
				idx = i
				break
			}
		}
		if idx >= 0 {
			s.entries[idx] = entry
		} else {
			s.entries = append(s.entries, entry)
		}
	}
	s.mu.Unlock()
	_ = s.save()
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

// Search returns the entries most relevant to query, best match first.
// An entry scores by how strongly its topic/tags appear *in the query* — this
// is the natural direction: the user's message mentions a keyword the entry is
// tagged with. (Matching the other way — entry contains the whole query — never
// fires for real questions and was a long-standing bug.)
func (s *Store) Search(query string, maxResults int) []ai.Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	lower := strings.ToLower(query)

	type scored struct {
		entry ai.Entry
		score int
	}
	var matches []scored

	for _, e := range s.entries {
		score := 0
		if t := strings.ToLower(e.Topic); t != "" && strings.Contains(lower, t) {
			score += 10
		}
		for _, tag := range e.Tags {
			if t := strings.ToLower(tag); t != "" && strings.Contains(lower, t) {
				score += 5
			}
		}
		if score > 0 {
			matches = append(matches, scored{e, score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	if len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	results := make([]ai.Entry, len(matches))
	for i, m := range matches {
		results[i] = m.entry
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

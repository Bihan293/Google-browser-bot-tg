package store

import (
	"sync"
	"time"
)

// SearchKind enumerates the kind of search a user performed.
type SearchKind string

const (
	KindWeb    SearchKind = "web"
	KindImages SearchKind = "img"
	KindVideos SearchKind = "vid"
	KindNews   SearchKind = "news"
)

// Session stores the last search context for a user so we can paginate
// and open individual results without recomputing the query each time.
type Session struct {
	Query     string
	Kind      SearchKind
	Page      int
	UpdatedAt time.Time
	// Cached results (URLs / titles) per kind; key = page index
	WebResults   [][]WebItem
	ImageResults [][]ImageItem
	VideoResults [][]VideoItem
	NewsResults  [][]NewsItem
}

// WebItem is a simple web search result.
type WebItem struct {
	Title   string
	URL     string
	Snippet string
}

// ImageItem is a single image search result.
type ImageItem struct {
	Title    string
	ImageURL string
	Source   string
}

// VideoItem is a video result (YouTube etc.)
type VideoItem struct {
	Title    string
	URL      string
	Author   string
	Duration string
	Thumb    string
}

// NewsItem is a news/article result.
type NewsItem struct {
	Title   string
	URL     string
	Source  string
	Snippet string
	Date    string
}

// Store is an in-memory thread-safe session store keyed by chat id.
type Store struct {
	mu       sync.RWMutex
	sessions map[int64]*Session
	ttl      time.Duration
}

// New creates a new Store with the given TTL for sessions.
func New(ttl time.Duration) *Store {
	s := &Store{
		sessions: make(map[int64]*Session),
		ttl:      ttl,
	}
	go s.gcLoop()
	return s
}

// Get returns the session for the chat or nil.
func (s *Store) Get(chatID int64) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[chatID]
}

// Set saves a session for the chat.
func (s *Store) Set(chatID int64, sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.UpdatedAt = time.Now()
	s.sessions[chatID] = sess
}

// Update mutates a session (creating it if necessary).
func (s *Store) Update(chatID int64, fn func(sess *Session)) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[chatID]
	if !ok {
		sess = &Session{}
		s.sessions[chatID] = sess
	}
	fn(sess)
	sess.UpdatedAt = time.Now()
	return sess
}

// Delete removes the session for a chat.
func (s *Store) Delete(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, chatID)
}

func (s *Store) gcLoop() {
	if s.ttl <= 0 {
		return
	}
	t := time.NewTicker(s.ttl / 2)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-s.ttl)
		s.mu.Lock()
		for id, sess := range s.sessions {
			if sess.UpdatedAt.Before(cutoff) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

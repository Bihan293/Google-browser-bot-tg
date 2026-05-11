package store

import (
	"crypto/sha1"
	"encoding/hex"
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
	NSFW      bool
	UpdatedAt time.Time
	// Cached results (URLs / titles) per kind; key = page index
	WebResults   [][]WebItem
	ImageResults [][]ImageItem
	VideoResults [][]VideoItem
	NewsResults  [][]NewsItem

	// Links found on the last opened page, addressable by callback.
	OpenedURL   string
	PageLinks   []PageLink
	PageImages  []string
	PageVideos  []string
	PageTitle   string

	// LinkMap maps short ids (8 hex chars) to full URLs the bot has
	// shown to the user — so callbacks like o|<id> can open them.
	LinkMap map[string]string
}

// PageLink — гиперссылка, найденная на открытой странице.
type PageLink struct {
	Text string
	URL  string
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
	PageURL  string
}

// VideoItem is a video result (YouTube etc.)
type VideoItem struct {
	Title    string
	URL      string
	Author   string
	Duration string
	Thumb    string
	VideoID  string // YouTube id, if known
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
		sess = &Session{LinkMap: map[string]string{}}
		s.sessions[chatID] = sess
	}
	if sess.LinkMap == nil {
		sess.LinkMap = map[string]string{}
	}
	fn(sess)
	sess.UpdatedAt = time.Now()
	return sess
}

// RegisterURL puts a URL into the session's link map and returns a short id
// that can be embedded into Telegram callback_data (< 64 bytes total).
func (s *Store) RegisterURL(chatID int64, rawURL string) string {
	if rawURL == "" {
		return ""
	}
	id := shortID(rawURL)
	s.Update(chatID, func(sess *Session) {
		sess.LinkMap[id] = rawURL
	})
	return id
}

// ResolveURL returns a previously registered URL by its short id, or "".
func (s *Store) ResolveURL(chatID int64, id string) string {
	sess := s.Get(chatID)
	if sess == nil || sess.LinkMap == nil {
		return ""
	}
	return sess.LinkMap[id]
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

func shortID(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:4]) // 8 hex chars
}

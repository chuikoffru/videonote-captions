package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

type pendingVideo struct {
	Request                             ProcessingRequest
	Duration                            int
	ExpiresAt                           time.Time
	Processing, AwaitingTrim, Cancelled bool
}

type pendingVideoStore struct {
	mu      sync.Mutex
	entries map[string]pendingVideo
	now     func() time.Time
}

func newPendingVideoStore() *pendingVideoStore {
	return &pendingVideoStore{entries: make(map[string]pendingVideo), now: time.Now}
}

func (s *pendingVideoStore) Put(req ProcessingRequest, duration int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	perUser := 0
	for key, p := range s.entries {
		if !p.ExpiresAt.After(now) {
			delete(s.entries, key)
		} else if p.Request.OwnerID == req.OwnerID {
			perUser++
		}
	}
	if perUser >= 10 || len(s.entries) >= 10000 {
		return "", errors.New("too many pending menus")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(random[:])
	req.PendingToken = token
	s.entries[token] = pendingVideo{Request: req, Duration: duration, ExpiresAt: now.Add(30 * time.Minute)}
	return token, nil
}

func (s *pendingVideoStore) Select(token string, ownerID, chatID int64, action string) (pendingVideo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.entries[token]
	if !ok || !p.ExpiresAt.After(s.now()) {
		delete(s.entries, token)
		return pendingVideo{}, errors.New("expired menu")
	}
	if p.Request.OwnerID != ownerID || p.Request.ChatID != chatID || p.Processing {
		return pendingVideo{}, errors.New("unavailable menu")
	}
	switch action {
	case "cancel":
		delete(s.entries, token)
		p.Cancelled = true
		return p, nil
	case "plain", "captions":
		p.Request.Mode = action
	case "trim":
		if !p.AwaitingTrim || p.Request.Mode == "" {
			return pendingVideo{}, errors.New("trim without selection")
		}
		p.Request.TrimConfirmed = true
	default:
		return pendingVideo{}, errors.New("unknown action")
	}
	p.AwaitingTrim = p.Duration > 60 && !p.Request.TrimConfirmed
	p.Processing = !p.AwaitingTrim
	s.entries[token] = p
	return p, nil
}

func (s *pendingVideoStore) RequireTrim(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.entries[token]; ok {
		p.Duration = 61
		p.Processing = false
		p.AwaitingTrim = true
		p.Request.TrimConfirmed = false
		s.entries[token] = p
	}
}
func (s *pendingVideoStore) Reset(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.entries[token]; ok {
		p.Processing = false
		s.entries[token] = p
	}
}
func (s *pendingVideoStore) Complete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.entries[token]; ok && p.Processing {
		delete(s.entries, token)
	}
}

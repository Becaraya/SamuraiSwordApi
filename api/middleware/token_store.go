package middleware

import (
	"sync"
	"time"
)

type TokenStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func NewTokenStore() *TokenStore {
	store := &TokenStore{
		tokens: make(map[string]time.Time),
	}
	go store.cleanupExpiredTokens()
	return store
}

func (s *TokenStore) AddToken(username string, expiry time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[username] = expiry
}

func (s *TokenStore) GetTokens() map[string]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens
}

func (s *TokenStore) cleanupExpiredTokens() {
	for {
		time.Sleep(1 * time.Hour) // Run cleanup every hour
		s.mu.Lock()
		for username, expiry := range s.tokens {
			if time.Now().After(expiry) {
				delete(s.tokens, username)
			}
		}
		s.mu.Unlock()
	}
}

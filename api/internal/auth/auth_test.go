package auth

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryRevocations struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func (r *memoryRevocations) RevokeToken(_ context.Context, hash string, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[hash] = exp
	return nil
}

func (r *memoryRevocations) ActiveRevokedTokens(_ context.Context) (map[string]time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]time.Time{}
	for hash, exp := range r.m {
		if exp.After(time.Now()) {
			out[hash] = exp
		}
	}
	return out, nil
}

func (r *memoryRevocations) CleanupRevokedTokens(_ context.Context) error { return nil }

func TestRevocationSurvivesServiceRestart(t *testing.T) {
	repo := &memoryRevocations{m: map[string]time.Time{}}
	secret := []byte("0123456789abcdef0123456789abcdef")
	first, err := New(secret, time.Hour, "admin", "password", "encryption", repo)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := first.Login("admin", "password")
	if err != nil {
		t.Fatal(err)
	}
	first.Revoke(token)

	second, err := New(secret, time.Hour, "admin", "password", "encryption", repo)
	if err != nil {
		t.Fatal(err)
	}
	if !second.isRevoked(token) {
		t.Fatal("restartdan keyin ham token bekor qilingan bo'lishi kerak")
	}
}

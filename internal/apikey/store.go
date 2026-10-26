// Package apikey provides persistent, hashed API key management.
package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const FileName = "api_keys.json"

type Record struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Hash      string     `json:"hash"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type Public struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
type Store struct {
	mu   sync.RWMutex
	path string
	keys []Record
}

func New(path string) (*Store, error) {
	s := &Store{path: path}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) > 0 {
		if err = json.Unmarshal(b, &s.keys); err != nil {
			return nil, fmt.Errorf("decode api keys: %w", err)
		}
	}
	return s, nil
}
func (s *Store) Create(name string) (Public, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "API key"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Public{}, "", err
	}
	token := "hme_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	now := time.Now().UTC()
	r := Record{ID: "key_" + hex.EncodeToString(sum[:6]), Name: name, Hash: hex.EncodeToString(sum[:]), CreatedAt: now}
	s.mu.Lock()
	s.keys = append(s.keys, r)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Public{}, "", err
	}
	return public(r), token, nil
}
func (s *Store) List() []Public {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Public, 0, len(s.keys))
	for _, r := range s.keys {
		out = append(out, public(r))
	}
	return out
}
func (s *Store) Revoke(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys {
		if s.keys[i].ID == id && s.keys[i].RevokedAt == nil {
			t := time.Now().UTC()
			s.keys[i].RevokedAt = &t
			_ = s.saveLocked()
			return true
		}
	}
	return false
}
func (s *Store) Authenticate(token string) (Public, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Public{}, false
	}
	sum := sha256.Sum256([]byte(token))
	h := hex.EncodeToString(sum[:])
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.keys {
		if r.RevokedAt == nil && subtle.ConstantTimeCompare([]byte(r.Hash), []byte(h)) == 1 {
			return public(r), true
		}
	}
	return Public{}, false
}
func public(r Record) Public {
	return Public{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, RevokedAt: r.RevokedAt}
}
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.keys, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

var ErrNotFound = errors.New("api key not found")

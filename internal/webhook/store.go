package webhook

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const FileName = "webhooks.json"

type Webhook struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	Secret       string            `json:"secret,omitempty"`
	Enabled      bool              `json:"enabled"`
	Events       []string          `json:"events,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyTemplate string            `json:"body_template,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
type Public struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	SecretSet    bool              `json:"secret_set"`
	Enabled      bool              `json:"enabled"`
	Events       []string          `json:"events,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	BodyTemplate string            `json:"body_template,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
type Input struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	URL          string            `json:"url"`
	Secret       *string           `json:"secret"`
	Enabled      *bool             `json:"enabled"`
	Events       []string          `json:"events"`
	Headers      map[string]string `json:"headers"`
	BodyTemplate string            `json:"body_template"`
}
type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Webhook
}

func New(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]Webhook{}}
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
	var a []Webhook
	if err = json.Unmarshal(b, &a); err != nil {
		return nil, err
	}
	for _, w := range a {
		s.items[w.ID] = w
	}
	return s, nil
}
func (s *Store) List() []Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Webhook, 0, len(s.items))
	for _, w := range s.items {
		out = append(out, w)
	}
	return out
}
func (s *Store) Get(id string) (Webhook, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.items[id]
	return w, ok
}
func (s *Store) Create(in Input) (Webhook, error) {
	if err := validate(in); err != nil {
		return Webhook{}, err
	}
	id := newID()
	now := time.Now().UTC()
	w := Webhook{ID: id, Name: in.Name, Type: in.Type, URL: in.URL, Enabled: true, CreatedAt: now, UpdatedAt: now, Events: clean(in.Events), Headers: in.Headers, BodyTemplate: strings.TrimSpace(in.BodyTemplate)}
	if in.Secret != nil {
		w.Secret = *in.Secret
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = w
	return w, s.saveLocked()
}
func (s *Store) Update(id string, in Input) (Webhook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.items[id]
	if !ok {
		return Webhook{}, os.ErrNotExist
	}
	if in.Name != "" {
		w.Name = in.Name
	}
	if in.Type != "" {
		w.Type = in.Type
	}
	if in.URL != "" {
		w.URL = in.URL
	}
	if in.Secret != nil {
		w.Secret = *in.Secret
	}
	if in.Enabled != nil {
		w.Enabled = *in.Enabled
	}
	if in.Events != nil {
		w.Events = clean(in.Events)
	}
	if in.Headers != nil {
		w.Headers = in.Headers
	}
	if in.BodyTemplate != "" {
		w.BodyTemplate = strings.TrimSpace(in.BodyTemplate)
	}
	if err := validateInput(w); err != nil {
		return Webhook{}, err
	}
	w.UpdatedAt = time.Now().UTC()
	s.items[id] = w
	return w, s.saveLocked()
}
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return s.saveLocked() == nil
}
func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	a := make([]Webhook, 0, len(s.items))
	for _, w := range s.items {
		a = append(a, w)
	}
	b, _ := json.MarshalIndent(a, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
func PublicView(w Webhook) Public {
	return Public{ID: w.ID, Name: w.Name, Type: w.Type, URL: w.URL, SecretSet: w.Secret != "", Enabled: w.Enabled, Events: w.Events, Headers: w.Headers, BodyTemplate: w.BodyTemplate, CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt}
}
func validate(in Input) error {
	return validateInput(Webhook{Type: in.Type, URL: in.URL, Name: in.Name})
}
func validateInput(w Webhook) error {
	if strings.TrimSpace(w.Name) == "" || len(w.Name) > 100 {
		return errors.New("name is required")
	}
	if w.Type != "telegram" && w.Type != "discord" && w.Type != "bark" && w.Type != "custom" {
		return errors.New("unsupported webhook type")
	}
	if w.URL == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(w.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("webhook URL must use https")
	}
	return nil
}
func clean(a []string) []string {
	r := []string{}
	for _, v := range a {
		if v != "" {
			r = append(r, v)
		}
	}
	return r
}
func newID() string { b := make([]byte, 8); _, _ = rand.Read(b); return "wh_" + hex.EncodeToString(b) }

func (s *Store) Dispatch(event string, payload any) error {
	envelope := map[string]any{"event": event, "payload": payload, "timestamp": time.Now().UTC()}
	b, _ := json.Marshal(envelope)
	var first error
	for _, w := range s.List() {
		if !w.Enabled || !matches(w.Events, event) {
			continue
		}
		if err := send(w, b, envelope); err != nil && first == nil {
			first = err
		}
	}
	return first
}
func matches(es []string, e string) bool {
	if len(es) == 0 {
		return true
	}
	for _, x := range es {
		if x == "*" || x == e {
			return true
		}
	}
	return false
}
func send(w Webhook, b []byte, envelope map[string]any) error {
	target := w.URL
	body := b
	if w.BodyTemplate != "" {
		body = []byte(renderTemplate(w.BodyTemplate, envelope))
	} else if w.Type == "telegram" {
		body = []byte(fmt.Sprintf(`{"text":%q}`, string(b)))
	} else if w.Type == "discord" {
		body = []byte(fmt.Sprintf(`{"content":%q}`, string(b)))
	}
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	if w.Secret != "" {
		req.Header.Set("X-Webhook-Secret", w.Secret)
	}
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}

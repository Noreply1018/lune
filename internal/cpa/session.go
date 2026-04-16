package cpa

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lune/internal/store"
)

type LoginSession struct {
	ID              string         `json:"id"`
	ServiceID       int64          `json:"service_id"`
	PoolID          int64          `json:"pool_id"`
	Provider        string         `json:"provider"`
	Status          string         `json:"status"` // pending, authorized, succeeded, expired, failed, cancelled
	VerificationURI string         `json:"verification_uri,omitempty"`
	UserCode        string         `json:"user_code,omitempty"`
	ExpiresAt       time.Time      `json:"expires_at"`
	PollInterval    int            `json:"poll_interval_seconds"`
	DeviceAuthID    string         `json:"-"` // stores device_code / device_auth_id, never exposed to client
	ErrorCode       string         `json:"error_code,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	AccountID       *int64         `json:"account_id,omitempty"`
	Account         *store.Account `json:"account,omitempty"`
	CancelFunc      func()         `json:"-"`
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*LoginSession
	path     string
}

func NewSessionStore(path string) *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*LoginSession),
		path:     path,
	}
	s.load()
	return s
}

func (s *SessionStore) CreateSession(serviceID int64, provider string, dcr *DeviceCodeResponse) (*LoginSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.normalizeLocked()

	if sess := s.getActiveSessionLocked(serviceID); sess != nil {
		return nil, fmt.Errorf("active session already exists for this service")
	}

	id := generateSessionID()
	session := &LoginSession{
		ID:              id,
		ServiceID:       serviceID,
		Provider:        provider,
		Status:          "pending",
		VerificationURI: dcr.VerificationURI,
		UserCode:        dcr.UserCode,
		ExpiresAt:       time.Now().Add(time.Duration(dcr.ExpiresIn) * time.Second),
		PollInterval:    dcr.Interval,
		DeviceAuthID:    dcr.DeviceCode,
	}

	s.sessions[id] = session
	s.saveLocked()
	return session, nil
}

func (s *SessionStore) GetSession(id string) *LoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()
	return s.sessions[id]
}

func (s *SessionStore) GetActiveSession(serviceID int64) *LoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()
	return s.getActiveSessionLocked(serviceID)
}

func (s *SessionStore) ListActiveSessions() []*LoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()

	var sessions []*LoginSession
	for _, sess := range s.sessions {
		if isActiveStatus(sess.Status) {
			sessions = append(sessions, sess)
		}
	}
	return sessions
}

func (s *SessionStore) CancelSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.normalizeLocked()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	if !isActiveStatus(sess.Status) {
		return fmt.Errorf("session is not active")
	}
	sess.Status = "cancelled"
	if sess.CancelFunc != nil {
		sess.CancelFunc()
	}
	s.saveLocked()
	return nil
}

func (s *SessionStore) UpdateStatus(id, status, errorCode, errorMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = status
		sess.ErrorCode = errorCode
		sess.ErrorMessage = errorMessage
		s.saveLocked()
	}
}

func (s *SessionStore) CompleteSession(id string, accountID int64, account *store.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = "succeeded"
		sess.AccountID = &accountID
		sess.Account = account
		s.saveLocked()
	}
}

func (s *SessionStore) UpdatePoolID(id string, poolID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.PoolID = poolID
		s.saveLocked()
	}
}

func (s *SessionStore) getActiveSessionLocked(serviceID int64) *LoginSession {
	for _, sess := range s.sessions {
		if sess.ServiceID == serviceID && isActiveStatus(sess.Status) {
			return sess
		}
	}
	return nil
}

func (s *SessionStore) normalizeLocked() {
	now := time.Now()
	changed := false
	for _, sess := range s.sessions {
		if isActiveStatus(sess.Status) && !sess.ExpiresAt.IsZero() && now.After(sess.ExpiresAt) {
			sess.Status = "expired"
			sess.ErrorCode = "expired_token"
			sess.ErrorMessage = "Device code expired"
			changed = true
		}
	}
	if changed {
		s.saveLocked()
	}
}

func (s *SessionStore) load() {
	if s.path == "" {
		return
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}

	var sessions map[string]*LoginSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		return
	}

	s.mu.Lock()
	s.sessions = sessions
	s.normalizeLocked()
	s.mu.Unlock()
}

func (s *SessionStore) saveLocked() {
	if s.path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}

func isActiveStatus(status string) bool {
	return status == "pending" || status == "authorized"
}

func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("login_%d", time.Now().UnixNano())
	}
	return "login_" + hex.EncodeToString(b)
}

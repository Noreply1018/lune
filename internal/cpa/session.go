package cpa

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"lune/internal/store"
)

type LoginSession struct {
	ID              string         `json:"id"`
	ServiceID       int64          `json:"service_id"`
	Status          string         `json:"status"` // pending, authorized, succeeded, expired, failed, cancelled
	VerificationURI string         `json:"verification_uri,omitempty"`
	UserCode        string         `json:"user_code,omitempty"`
	ExpiresAt       time.Time      `json:"expires_at"`
	PollInterval    int            `json:"poll_interval_seconds"`
	DeviceCode      string         `json:"-"` // never exposed to client
	ErrorCode       string         `json:"error_code,omitempty"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	AccountID       *int64         `json:"account_id,omitempty"`
	Account         *store.Account `json:"account,omitempty"`
	CancelFunc      func()         `json:"-"`
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*LoginSession
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*LoginSession),
	}
}

func (s *SessionStore) CreateSession(serviceID int64, dcr *DeviceCodeResponse) (*LoginSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// check for existing active session for this service
	for _, sess := range s.sessions {
		if sess.ServiceID == serviceID && (sess.Status == "pending" || sess.Status == "authorized") {
			return nil, fmt.Errorf("active session already exists for this service")
		}
	}

	id := generateSessionID()
	session := &LoginSession{
		ID:              id,
		ServiceID:       serviceID,
		Status:          "pending",
		VerificationURI: dcr.VerificationURI,
		UserCode:        dcr.UserCode,
		ExpiresAt:       time.Now().Add(time.Duration(dcr.ExpiresIn) * time.Second),
		PollInterval:    dcr.Interval,
		DeviceCode:      dcr.DeviceCode,
	}

	s.sessions[id] = session
	return session, nil
}

func (s *SessionStore) GetSession(id string) *LoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

func (s *SessionStore) CancelSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return fmt.Errorf("session not found")
	}
	if sess.Status != "pending" && sess.Status != "authorized" {
		return fmt.Errorf("session is not active")
	}
	sess.Status = "cancelled"
	if sess.CancelFunc != nil {
		sess.CancelFunc()
	}
	return nil
}

func (s *SessionStore) UpdateStatus(id, status, errorCode, errorMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = status
		sess.ErrorCode = errorCode
		sess.ErrorMessage = errorMessage
	}
}

func (s *SessionStore) CompleteSession(id string, accountID int64, account *store.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[id]; ok {
		sess.Status = "succeeded"
		sess.AccountID = &accountID
		sess.Account = account
	}
}

func generateSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "login_" + hex.EncodeToString(b)
}

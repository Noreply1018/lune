package execution

import (
	"io"
	"net/http"
	"time"

	"lune/internal/config"
)

type Request struct {
	RequestID       string
	Endpoint        string
	Method          string
	ModelAlias      string
	RawBody         []byte
	Payload         map[string]any
	Headers         http.Header
	Stream          bool
	AccessTokenName string
}

type Plan struct {
	PoolID       string
	PlatformID   string
	AccountID    string
	TargetModel  string
	AdapterID    string
	AttemptIndex int
}

type PreparedExecution struct {
	Request     Request
	Plan        Plan
	Platform    config.Platform
	Account     config.Account
	TargetModel string
	RawBody     []byte
	Payload     map[string]any
	Headers     http.Header
}

type RawResult struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type GatewayResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

type Outcome string

const (
	OutcomeSuccess          Outcome = "success"
	OutcomeRetryableFailure Outcome = "retryable_failure"
	OutcomeFinalFailure     Outcome = "final_failure"
	OutcomeNotImplemented   Outcome = "not_implemented"
)

type Record struct {
	RequestID        string
	CreatedAt        time.Time
	AccessTokenName  string
	Method           string
	Endpoint         string
	ModelAlias       string
	Stream           bool
	PoolID           string
	PlatformID       string
	AccountID        string
	TargetModel      string
	AttemptCount     int
	StatusCode       int
	LatencyMS        int64
	Success          bool
	ErrorMessage     string
	APICostUnits     int64
	AccountCostUnits int64
	AccountCostType  string
	LastSuccessAt    *time.Time
	LastError        string
	CooldownUntil    *time.Time
}

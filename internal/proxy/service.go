package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	accountadapter "lune/internal/adapter/account"
	"lune/internal/auth"
	"lune/internal/config"
	"lune/internal/execution"
	"lune/internal/metrics"
	"lune/internal/platform"
	"lune/internal/router"
	"lune/internal/runtimeconfig"
	"lune/internal/store"
)

var (
	ErrPlatformNotFound = errors.New("platform not found")
	ErrAccountNotFound  = errors.New("account not found")
)

type Service struct {
	config   *runtimeconfig.Manager
	registry *platform.Registry
	adapters *accountadapter.Registry
	ledger   store.LedgerStore
	store    *store.Store
	metrics  *metrics.Collector
}

func New(cfgManager *runtimeconfig.Manager, registry *platform.Registry, adapters *accountadapter.Registry, st *store.Store, metricCollector *metrics.Collector) *Service {
	return &Service{
		config:   cfgManager,
		registry: registry,
		adapters: adapters,
		ledger:   st,
		store:    st,
		metrics:  metricCollector,
	}
}

func buildPlatformMap(platforms []config.Platform) map[string]config.Platform {
	out := make(map[string]config.Platform, len(platforms))
	for _, platform := range platforms {
		out[platform.ID] = platform
	}
	return out
}

func buildAccountMap(accounts []config.Account) map[string]config.Account {
	out := make(map[string]config.Account, len(accounts))
	for _, account := range accounts {
		out[account.ID] = account
	}
	return out
}

func (s *Service) currentConfig() config.Config {
	if s.config == nil {
		return config.Config{}
	}
	return s.config.Current()
}

func (s *Service) ListModels() []config.ModelRoute {
	return s.currentConfig().Models
}

func (s *Service) ChatCompletions(w http.ResponseWriter, r *http.Request) error {
	return s.proxyOpenAIEndpoint(w, r, "/v1/chat/completions")
}

func (s *Service) Responses(w http.ResponseWriter, r *http.Request) error {
	return s.proxyOpenAIEndpoint(w, r, "/v1/responses")
}

func (s *Service) Embeddings(w http.ResponseWriter, r *http.Request) error {
	return s.proxyOpenAIEndpoint(w, r, "/v1/embeddings")
}

func (s *Service) ImagesGenerations(w http.ResponseWriter, r *http.Request) error {
	return s.proxyOpenAIEndpoint(w, r, "/v1/images/generations")
}

func (s *Service) proxyOpenAIEndpoint(w http.ResponseWriter, r *http.Request, endpoint string) error {
	start := time.Now().UTC()

	req, err := s.buildExecutionRequest(r, endpoint, start)
	if err != nil {
		return err
	}
	if err := s.ensureQuota(r.Context()); err != nil {
		return err
	}

	cfg := s.currentConfig()
	plans, err := router.New(cfg).Resolve(req)
	if err != nil {
		switch {
		case errors.Is(err, router.ErrModelNotFound):
			return newProxyError(http.StatusBadRequest, "unknown model alias", err)
		case errors.Is(err, router.ErrNoAvailableAccount):
			return newProxyError(http.StatusBadGateway, "no runnable account available", err)
		default:
			return newProxyError(http.StatusInternalServerError, "resolve model route failed", err)
		}
	}

	var lastErr error
	var lastRecord execution.Record
	var sawNotImplemented bool

	for _, plan := range plans {
		record, response, outcome, err := s.executePlan(r.Context(), req, plan, start)
		if outcome == execution.OutcomeSuccess {
			copyGatewayResponse(w, response, cfg.Server.StreamHeartbeat)
			if err := s.consumeQuota(r.Context()); err != nil {
				return newProxyError(http.StatusInternalServerError, "consume token quota failed", err)
			}
			s.recordMetrics(record)
			if s.ledger != nil {
				_ = s.ledger.RecordSuccess(r.Context(), record)
			}
			return nil
		}

		lastErr = err
		lastRecord = record
		if outcome == execution.OutcomeNotImplemented {
			sawNotImplemented = true
			break
		}
		if outcome == execution.OutcomeRetryableFailure {
			if s.ledger != nil {
				_ = s.ledger.RecordAttempt(r.Context(), record)
			}
			continue
		}
		break
	}

	failureStatus := http.StatusBadGateway
	if sawNotImplemented {
		failureStatus = http.StatusNotImplemented
	}
	if lastRecord.StatusCode != 0 {
		failureStatus = lastRecord.StatusCode
	}
	lastRecord.StatusCode = failureStatus
	lastRecord.Success = false
	lastRecord.APICostUnits = 0
	lastRecord.AccountCostUnits = 0
	lastRecord.AccountCostType = "request"
	lastRecord.LatencyMS = time.Since(start).Milliseconds()
	lastRecord.CreatedAt = start
	if lastRecord.ErrorMessage == "" {
		lastRecord.ErrorMessage = errString(lastErr)
	}

	s.recordMetrics(lastRecord)
	if s.ledger != nil {
		_ = s.ledger.RecordFailure(r.Context(), lastRecord)
	}

	message := "all account execution attempts failed"
	if sawNotImplemented {
		message = "no account adapter is implemented for this platform"
	}
	return newProxyError(failureStatus, message, lastErr)
}

func (s *Service) buildExecutionRequest(r *http.Request, endpoint string, startedAt time.Time) (execution.Request, error) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		return execution.Request{}, fmt.Errorf("read body: %w", err)
	}

	payload := make(map[string]any)
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return execution.Request{}, newProxyError(http.StatusBadRequest, "invalid json body", err)
	}

	modelAlias, _ := payload["model"].(string)
	if strings.TrimSpace(modelAlias) == "" {
		return execution.Request{}, newProxyError(http.StatusBadRequest, "model is required", nil)
	}

	return execution.Request{
		RequestID:       fmt.Sprintf("%d", startedAt.UnixNano()),
		Endpoint:        endpoint,
		Method:          r.Method,
		ModelAlias:      modelAlias,
		RawBody:         rawBody,
		Payload:         payload,
		Headers:         r.Header.Clone(),
		Stream:          payload["stream"] == true,
		AccessTokenName: auth.AccessTokenNameFromContext(r.Context()),
	}, nil
}

func (s *Service) executePlan(ctx context.Context, req execution.Request, plan execution.Plan, startedAt time.Time) (execution.Record, *execution.GatewayResponse, execution.Outcome, error) {
	record := execution.Record{
		RequestID:       req.RequestID,
		CreatedAt:       startedAt,
		AccessTokenName: req.AccessTokenName,
		Method:          req.Method,
		Endpoint:        req.Endpoint,
		ModelAlias:      req.ModelAlias,
		Stream:          req.Stream,
		PoolID:          plan.PoolID,
		PlatformID:      plan.PlatformID,
		AccountID:       plan.AccountID,
		TargetModel:     plan.TargetModel,
		AttemptCount:    plan.AttemptIndex + 1,
		AccountCostType: "request",
	}

	cfg := s.currentConfig()
	platformCfg, ok := buildPlatformMap(cfg.Platforms)[plan.PlatformID]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrPlatformNotFound, plan.PlatformID)
		record.ErrorMessage = err.Error()
		return record, nil, execution.OutcomeFinalFailure, err
	}
	if !platformCfg.Enabled {
		err := fmt.Errorf("platform %s is disabled", platformCfg.ID)
		record.ErrorMessage = err.Error()
		return record, nil, execution.OutcomeFinalFailure, err
	}
	if s.registry != nil {
		if status, ok := s.registry.Status(platformCfg.ID); ok && !status.Healthy {
			err := fmt.Errorf("platform %s is unhealthy", platformCfg.ID)
			record.ErrorMessage = err.Error()
			return record, nil, execution.OutcomeFinalFailure, err
		}
	}

	accountCfg, ok := buildAccountMap(cfg.Accounts)[plan.AccountID]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrAccountNotFound, plan.AccountID)
		record.ErrorMessage = err.Error()
		return record, nil, execution.OutcomeFinalFailure, err
	}

	adapter, ok := s.adapters.ForPlatform(platformCfg)
	if !ok {
		adapterID := plan.AdapterID
		if adapterID == "" {
			adapterID = platformCfg.Adapter
		}
		err := fmt.Errorf("%w: %s", accountadapter.ErrAdapterNotImplemented, adapterID)
		record.ErrorMessage = err.Error()
		record.StatusCode = http.StatusNotImplemented
		record.LatencyMS = time.Since(startedAt).Milliseconds()
		record.LastError = record.ErrorMessage
		return record, nil, execution.OutcomeNotImplemented, err
	}

	prepared, err := adapter.Prepare(ctx, req, plan, platformCfg, accountCfg)
	if err != nil {
		record.ErrorMessage = errString(err)
		record.StatusCode = statusForOutcome(adapter.Classify(nil, err))
		record.LatencyMS = time.Since(startedAt).Milliseconds()
		record.LastError = record.ErrorMessage
		return record, nil, adapter.Classify(nil, err), err
	}

	rawResult, execErr := adapter.Execute(ctx, prepared)
	outcome := adapter.Classify(rawResult, execErr)
	if execErr != nil {
		record.ErrorMessage = errString(execErr)
		record.StatusCode = statusForOutcome(outcome)
		record.LatencyMS = time.Since(startedAt).Milliseconds()
		record.LastError = record.ErrorMessage
		return record, nil, outcome, execErr
	}

	response, err := adapter.Normalize(ctx, prepared, rawResult)
	if err != nil {
		outcome = adapter.Classify(rawResult, err)
		record.ErrorMessage = errString(err)
		record.StatusCode = statusForOutcome(outcome)
		record.LatencyMS = time.Since(startedAt).Milliseconds()
		record.LastError = record.ErrorMessage
		return record, nil, outcome, err
	}

	record.StatusCode = response.StatusCode
	record.LatencyMS = time.Since(startedAt).Milliseconds()

	if outcome == execution.OutcomeSuccess {
		now := time.Now().UTC()
		record.Success = true
		record.APICostUnits = 1
		record.AccountCostUnits = 1
		record.LastSuccessAt = &now
		record.LastError = ""
		return record, response, outcome, nil
	}

	record.ErrorMessage = fmt.Sprintf("account %s returned status %d", accountCfg.ID, response.StatusCode)
	if detail := summarizeGatewayFailure(response.Body); detail != "" {
		record.ErrorMessage += ": " + detail
	}
	record.LastError = record.ErrorMessage
	return record, response, outcome, errors.New(record.ErrorMessage)
}

func summarizeGatewayFailure(body io.ReadCloser) string {
	if body == nil {
		return ""
	}
	defer body.Close()

	raw, err := io.ReadAll(io.LimitReader(body, 2048))
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 400 {
		text = text[:400] + "..."
	}
	return text
}

func (s *Service) recordMetrics(record execution.Record) {
	if s.metrics != nil {
		s.metrics.Record(time.Duration(record.LatencyMS)*time.Millisecond, record.Success)
	}
}

func (s *Service) ensureQuota(ctx context.Context) error {
	if s.store == nil {
		return nil
	}

	tokenName := auth.AccessTokenNameFromContext(ctx)
	if tokenName == "" {
		return nil
	}

	account, allowed, err := s.store.CanConsume(ctx, tokenName)
	if err != nil {
		return newProxyError(http.StatusInternalServerError, "read token quota failed", err)
	}
	if !allowed {
		return newProxyError(http.StatusPaymentRequired, fmt.Sprintf("token quota exhausted: remaining=%d", account.RemainingCalls()), nil)
	}
	return nil
}

func (s *Service) consumeQuota(ctx context.Context) error {
	if s.store == nil {
		return nil
	}
	tokenName := auth.AccessTokenNameFromContext(ctx)
	if tokenName == "" {
		return nil
	}
	return s.store.ConsumeRequest(ctx, tokenName)
}

func copyGatewayResponse(w http.ResponseWriter, result *execution.GatewayResponse, heartbeatEnabled bool) {
	if result == nil {
		return
	}
	defer result.Body.Close()

	for key, values := range result.Header {
		if hopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(result.StatusCode)
	if strings.Contains(strings.ToLower(result.Header.Get("Content-Type")), "text/event-stream") {
		streamCopy(w, result.Body, heartbeatEnabled)
		return
	}
	_, _ = io.Copy(w, result.Body)
}

func statusForOutcome(outcome execution.Outcome) int {
	switch outcome {
	case execution.OutcomeNotImplemented:
		return http.StatusNotImplemented
	case execution.OutcomeRetryableFailure, execution.OutcomeFinalFailure:
		return http.StatusBadGateway
	default:
		return http.StatusOK
	}
}

func hopByHopHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

type ProxyError struct {
	Status  int
	Message string
	Err     error
}

func newProxyError(status int, message string, err error) *ProxyError {
	return &ProxyError{
		Status:  status,
		Message: message,
		Err:     err,
	}
}

func (e *ProxyError) Error() string {
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *ProxyError) Unwrap() error {
	return e.Err
}

func IsTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func streamCopy(w http.ResponseWriter, body io.Reader, heartbeatEnabled bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		_, _ = io.Copy(w, body)
		return
	}

	type chunk struct {
		data []byte
		err  error
	}

	reader := bufio.NewReader(body)
	chunks := make(chan chunk, 1)

	go func() {
		defer close(chunks)
		buffer := make([]byte, 2048)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buffer[:n])
				chunks <- chunk{data: data}
			}
			if err != nil {
				chunks <- chunk{err: err}
				return
			}
		}
	}()

	var ticker *time.Ticker
	if heartbeatEnabled {
		ticker = time.NewTicker(15 * time.Second)
		defer ticker.Stop()
	}

	for {
		select {
		case item, ok := <-chunks:
			if !ok {
				return
			}
			if len(item.data) > 0 {
				_, _ = w.Write(item.data)
				flusher.Flush()
			}
			if item.err != nil {
				return
			}
		case <-tickerChan(ticker):
			_, _ = w.Write([]byte(": heartbeat\n\n"))
			flusher.Flush()
		}
	}
}

func tickerChan(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

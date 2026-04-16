package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const SecretPlaceholder = "***"

type Result struct {
	OK              bool   `json:"ok"`
	UpstreamCode    string `json:"upstream_code"`
	UpstreamMessage string `json:"upstream_message"`
	LatencyMS       int64  `json:"latency_ms"`
	ResponseExcerpt string `json:"response_excerpt"`
}

type ChannelRuntime struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Config    json.RawMessage `json:"config"`
	TitleTpl  string          `json:"title_template"`
	BodyTpl   string          `json:"body_template"`
	Triggered string          `json:"triggered_by"`
}

type ChannelDriver interface {
	Type() string
	ValidateConfig(raw json.RawMessage) error
	Send(ctx context.Context, n Notification, cfg ChannelRuntime) (Result, error)
	SecretFields() []string
	DocsURL() string
}

type Registry struct {
	drivers map[string]ChannelDriver
}

func NewRegistry(drivers ...ChannelDriver) *Registry {
	r := &Registry{drivers: make(map[string]ChannelDriver)}
	for _, driver := range drivers {
		r.Register(driver)
	}
	return r
}

func (r *Registry) Register(driver ChannelDriver) {
	r.drivers[driver.Type()] = driver
}

func (r *Registry) Get(typ string) (ChannelDriver, bool) {
	driver, ok := r.drivers[typ]
	return driver, ok
}

func (r *Registry) MustGet(typ string) ChannelDriver {
	driver, ok := r.Get(typ)
	if !ok {
		panic("unknown notification driver: " + typ)
	}
	return driver
}

func (r *Registry) Types() []string {
	out := make([]string, 0, len(r.drivers))
	for typ := range r.drivers {
		out = append(out, typ)
	}
	return out
}

func (r *Registry) MaskConfig(typ string, raw json.RawMessage) (map[string]any, error) {
	cfg := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	driver, ok := r.Get(typ)
	if !ok {
		return cfg, nil
	}
	for _, key := range driver.SecretFields() {
		if value, ok := cfg[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			cfg[key] = SecretPlaceholder
		}
	}
	return cfg, nil
}

func (r *Registry) MergeConfig(typ string, existingRaw json.RawMessage, incoming map[string]any) (json.RawMessage, error) {
	existing := make(map[string]any)
	if len(existingRaw) > 0 {
		if err := json.Unmarshal(existingRaw, &existing); err != nil {
			return nil, err
		}
	}
	driver, _ := r.Get(typ)
	secretFields := map[string]struct{}{}
	if driver != nil {
		for _, key := range driver.SecretFields() {
			secretFields[key] = struct{}{}
		}
	}

	for key, value := range incoming {
		if stringValue, ok := value.(string); ok && stringValue == SecretPlaceholder {
			if _, isSecret := secretFields[key]; isSecret {
				continue
			}
		}
		existing[key] = value
	}
	encoded, err := json.Marshal(existing)
	if err != nil {
		return nil, err
	}
	if driver != nil {
		if err := driver.ValidateConfig(encoded); err != nil {
			return nil, err
		}
	}
	return encoded, nil
}

package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type ReplayBody struct {
	data []byte
	path string
	size int64
}

func NewReplayBody(r *http.Request, maxBytes, memoryBytes int64, tmpDir string) (*ReplayBody, error) {
	if maxBytes <= 0 {
		maxBytes = 1
	}
	if memoryBytes <= 0 {
		memoryBytes = maxBytes
	}
	limited := io.LimitReader(r.Body, maxBytes+1)
	data, err := io.ReadAll(io.LimitReader(limited, memoryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrBodyTooLarge
	}
	if int64(len(data)) <= memoryBytes {
		return &ReplayBody{data: data, size: int64(len(data))}, nil
	}

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("create gateway tmp dir: %w", err)
	}
	f, err := os.CreateTemp(tmpDir, "lune-body-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create replay file: %w", err)
	}
	path := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			f.Close()
			os.Remove(path)
		}
	}()
	written, err := f.Write(data)
	if err != nil {
		return nil, fmt.Errorf("write replay file: %w", err)
	}
	n, err := io.Copy(f, limited)
	if err != nil {
		return nil, fmt.Errorf("write replay file: %w", err)
	}
	size := int64(written) + n
	if size > maxBytes {
		return nil, ErrBodyTooLarge
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close replay file: %w", err)
	}
	cleanup = false
	return &ReplayBody{path: path, size: size}, nil
}

var ErrBodyTooLarge = errors.New("request body too large")

func (b *ReplayBody) Size() int64 {
	if b == nil {
		return 0
	}
	return b.size
}

func (b *ReplayBody) Storage() string {
	if b == nil || b.path == "" {
		return "memory"
	}
	return "disk"
}

func (b *ReplayBody) Reader() (io.ReadCloser, error) {
	if b.path == "" {
		return io.NopCloser(bytes.NewReader(b.data)), nil
	}
	return os.Open(b.path)
}

func (b *ReplayBody) Bytes() []byte {
	return b.data
}

func (b *ReplayBody) Close() error {
	if b == nil || b.path == "" {
		return nil
	}
	err := os.Remove(b.path)
	b.path = ""
	return err
}

func CleanupReplayDir(tmpDir string) error {
	entries, err := os.ReadDir(tmpDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".tmp" {
			_ = os.Remove(filepath.Join(tmpDir, entry.Name()))
		}
	}
	return nil
}

type requestEnvelope struct {
	Model  string
	Stream bool
}

func ParseRequestEnvelope(body *ReplayBody) (requestEnvelope, error) {
	reader, err := body.Reader()
	if err != nil {
		return requestEnvelope{}, err
	}
	defer reader.Close()

	dec := json.NewDecoder(reader)
	tok, err := dec.Token()
	if err != nil {
		return requestEnvelope{}, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return requestEnvelope{}, fmt.Errorf("request body must be a JSON object")
	}
	var env requestEnvelope
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return requestEnvelope{}, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return requestEnvelope{}, fmt.Errorf("invalid JSON object key")
		}
		switch key {
		case "model":
			if err := dec.Decode(&env.Model); err != nil {
				return requestEnvelope{}, fmt.Errorf("model must be a string")
			}
		case "stream":
			if err := dec.Decode(&env.Stream); err != nil {
				return requestEnvelope{}, fmt.Errorf("stream must be a boolean")
			}
		default:
			if err := skipJSONValue(dec); err != nil {
				return requestEnvelope{}, err
			}
		}
	}
	if _, err := dec.Token(); err != nil {
		return requestEnvelope{}, err
	}
	return env, nil
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		for dec.More() {
			if _, err := dec.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return nil
	}
}

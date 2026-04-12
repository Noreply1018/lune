package backendproxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"lune/internal/runtimeconfig"
)

// Handler proxies backend management requests through Lune. Requests are
// authenticated against Lune first, then Lune injects the backend admin token.
func Handler(cfg *runtimeconfig.Manager) http.Handler {
	proxy := &Proxy{
		cfg:     cfg,
		client:  &http.Client{},
		session: newAdminSession(&http.Client{}),
	}
	return http.HandlerFunc(proxy.ServeHTTP)
}

type Proxy struct {
	cfg     *runtimeconfig.Manager
	client  *http.Client
	session *adminSession
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := p.targetURL()
	if err != nil {
		http.Error(w, "bad upstream URL", http.StatusBadGateway)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read request body failed", http.StatusBadRequest)
		return
	}

	resp, err := p.do(r, target, body, true)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errBackendAdminCredsMissing) {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, err.Error(), status)
		return
	}
	defer resp.Body.Close()

	copyResponse(w, resp)
}

func (p *Proxy) do(r *http.Request, target *url.URL, body []byte, allowRetry bool) (*http.Response, error) {
	token, err := p.session.Token(r.Context(), target.String())
	if err != nil {
		return nil, err
	}

	req, err := p.newUpstreamRequest(r, target, body, token)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && allowRetry {
		resp.Body.Close()
		p.session.Invalidate(target.String())
		return p.do(r, target, body, false)
	}

	return resp, nil
}

func (p *Proxy) newUpstreamRequest(r *http.Request, target *url.URL, body []byte, token string) (*http.Request, error) {
	upstreamURL := *target
	upstreamURL.Path = joinURLPath(target.Path, strings.TrimPrefix(r.URL.Path, "/backend"))
	upstreamURL.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header = make(http.Header, len(r.Header))
	for k, values := range r.Header {
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Host = target.Host
	req.ContentLength = int64(len(body))

	return req, nil
}

func (p *Proxy) targetURL() (*url.URL, error) {
	upstream := p.cfg.Current().Server.UpstreamURL
	if upstream == "" {
		upstream = "http://localhost:3000"
	}
	return url.Parse(upstream)
}

func joinURLPath(basePath, requestPath string) string {
	basePath = strings.TrimRight(basePath, "/")
	if requestPath == "" {
		if basePath == "" {
			return "/"
		}
		return basePath
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}
	if basePath == "" {
		return requestPath
	}
	return basePath + requestPath
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(k, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

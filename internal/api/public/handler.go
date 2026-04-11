package public

import (
	"net/http"
	"time"

	"lune/internal/proxy"
	"lune/internal/runtimeconfig"
	"lune/internal/webutil"
)

type Handler struct {
	cfg   *runtimeconfig.Manager
	proxy *proxy.Service
}

func NewHandler(cfg *runtimeconfig.Manager, proxySvc *proxy.Service) *Handler {
	return &Handler{
		cfg:   cfg,
		proxy: proxySvc,
	}
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "lune",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.Current()
	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "ready",
		"platforms_loaded": len(cfg.Platforms),
		"accounts_loaded":  len(cfg.Accounts),
		"pools_loaded":     len(cfg.AccountPools),
		"models_loaded":    len(cfg.Models),
	})
}

func (h *Handler) Models(w http.ResponseWriter, r *http.Request) {
	cfg := h.cfg.Current()
	data := make([]map[string]any, 0, len(cfg.Models))
	for _, model := range h.proxy.ListModels() {
		data = append(data, map[string]any{
			"id":       model.Alias,
			"object":   "model",
			"owned_by": "lune",
		})
	}

	webutil.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	h.writeProxyError(w, h.proxy.ChatCompletions(w, r))
}

func (h *Handler) Responses(w http.ResponseWriter, r *http.Request) {
	h.writeProxyError(w, h.proxy.Responses(w, r))
}

func (h *Handler) Embeddings(w http.ResponseWriter, r *http.Request) {
	h.writeProxyError(w, h.proxy.Embeddings(w, r))
}

func (h *Handler) ImagesGenerations(w http.ResponseWriter, r *http.Request) {
	h.writeProxyError(w, h.proxy.ImagesGenerations(w, r))
}

func (h *Handler) writeProxyError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	status := http.StatusBadGateway
	if proxyErr, ok := err.(*proxy.ProxyError); ok {
		status = proxyErr.Status
	}

	webutil.WriteJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": err.Error(),
			"type":    "proxy_error",
		},
	})
}

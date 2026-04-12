package webutil

import (
	"encoding/json"
	"net/http"
	"strings"
)

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func BearerToken(header string) string {
	if header == "" {
		return ""
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

// Admin API response helpers

type adminErrorBody struct {
	Error adminErrorDetail `json:"error"`
}

type adminErrorDetail struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func WriteAdminError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, adminErrorBody{
		Error: adminErrorDetail{Message: message, Code: code},
	})
}

func WriteData(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, map[string]any{"data": data})
}

func WriteList(w http.ResponseWriter, data any, total int) {
	WriteJSON(w, http.StatusOK, map[string]any{"data": data, "total": total})
}

// Gateway error response (OpenAI format)

type gatewayErrorBody struct {
	Error gatewayErrorDetail `json:"error"`
}

type gatewayErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func WriteGatewayError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, gatewayErrorBody{
		Error: gatewayErrorDetail{
			Message: message,
			Type:    "gateway_error",
			Code:    code,
		},
	})
}

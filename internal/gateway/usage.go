package gateway

import "encoding/json"

type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

func ParseUsageFromBody(body []byte) Usage {
	var resp struct {
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Usage == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}
}

func ParseUsageFromSSEChunk(chunk []byte) Usage {
	return ParseUsageFromBody(chunk)
}

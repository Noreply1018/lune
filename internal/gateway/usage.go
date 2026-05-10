package gateway

import "encoding/json"

type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

func ParseUsageFromBody(body []byte) Usage {
	type usageFields struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		InputTokens      int64 `json:"input_tokens"`
		OutputTokens     int64 `json:"output_tokens"`
	}
	var resp struct {
		Usage    *usageFields `json:"usage"`
		Response *struct {
			Usage *usageFields `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Usage{}
	}
	usage := resp.Usage
	if usage == nil && resp.Response != nil {
		usage = resp.Response.Usage
	}
	if usage == nil {
		return Usage{}
	}
	input := usage.PromptTokens
	if input == 0 {
		input = usage.InputTokens
	}
	output := usage.CompletionTokens
	if output == 0 {
		output = usage.OutputTokens
	}
	return Usage{
		InputTokens:  input,
		OutputTokens: output,
	}
}

func ParseUsageFromSSEChunk(chunk []byte) Usage {
	return ParseUsageFromBody(chunk)
}

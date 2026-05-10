package gateway

import "testing"

func TestParseUsageFromBodySupportsChatCompletionsUsage(t *testing.T) {
	usage := ParseUsageFromBody([]byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))

	if usage.InputTokens != 11 || usage.OutputTokens != 7 {
		t.Fatalf("expected chat usage 11/7, got %+v", usage)
	}
}

func TestParseUsageFromBodySupportsResponsesUsage(t *testing.T) {
	usage := ParseUsageFromBody([]byte(`{"usage":{"input_tokens":356,"output_tokens":5,"total_tokens":361}}`))

	if usage.InputTokens != 356 || usage.OutputTokens != 5 {
		t.Fatalf("expected responses usage 356/5, got %+v", usage)
	}
}

func TestParseUsageFromSSEChunkSupportsResponsesCompletedUsage(t *testing.T) {
	chunk := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":355,"output_tokens":5,"total_tokens":360}}}`)
	usage := ParseUsageFromSSEChunk(chunk)

	if usage.InputTokens != 355 || usage.OutputTokens != 5 {
		t.Fatalf("expected responses completed usage 355/5, got %+v", usage)
	}
}

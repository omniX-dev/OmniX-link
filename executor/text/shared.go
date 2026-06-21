package executor

import "encoding/json"

// ReplaceModelField replaces the "model" field in a JSON request body.
func ReplaceModelField(body []byte, model string) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	raw["model"], _ = json.Marshal(model)
	result, _ := json.Marshal(raw)
	return result
}

// InjectStreamOptionsOpenAI injects stream_options.include_usage into an OpenAI request.
func InjectStreamOptionsOpenAI(body []byte) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body
	}
	if streamRaw, ok := raw["stream"]; ok {
		var stream bool
		if json.Unmarshal(streamRaw, &stream) == nil && stream {
			if _, exists := raw["stream_options"]; !exists {
				opts, _ := json.Marshal(map[string]bool{"include_usage": true})
				raw["stream_options"] = opts
			}
		}
	}
	result, _ := json.Marshal(raw)
	return result
}

// Package nanobanana implements ImageExecutor for Nano Banana 2 and Nanobanana Pro.
//
// API: https://api.nanobanana.ai/v1/images/generations (OpenAI-compatible)
// Models: nanobanana-2, nanobanana-pro
// TextToImage only — ImageToImage returns ErrNotSupported.
package nanobanana

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/image"
)

func init() {
	image.RegisterImage("nanobanana", &NanoBananaExecutor{})
}

// NanoBananaExecutor handles Nano Banana image generation (OpenAI-compatible API).
type NanoBananaExecutor struct {
	channel any
}

func (e *NanoBananaExecutor) Init(channel any) {
	e.channel = channel
}

func (e *NanoBananaExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "NanoBanana"
}

func (e *NanoBananaExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.nanobanana.ai"
}

func (e *NanoBananaExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *NanoBananaExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	// Nano Banana uses OpenAI-compatible /v1/images/generations
	body := map[string]any{
		"model":           req.Model,
		"prompt":          req.Prompt,
		"n":               req.N,
		"size":            req.Size,
		"quality":         req.Quality,
		"response_format": req.ResponseFormat,
	}
	if body["n"] == 0 {
		body["n"] = 1
	}
	if body["response_format"] == "" {
		body["response_format"] = "url"
	}

	// Extra passthrough
	for k, v := range req.Extra {
		if _, reserved := reservedFields[k]; !reserved {
			body[k] = v
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("nanobanana: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("nanobanana: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nanobanana: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL           string `json:"url,omitempty"`
			B64JSON       string `json:"b64_json,omitempty"`
			RevisedPrompt string `json:"revised_prompt,omitempty"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("nanobanana: unmarshal: %w", err)
	}

	task := &image.ImageTask{
		Status:    image.TaskStatusCompleted,
		CreatedAt: oaiResp.Created,
	}
	for i, d := range oaiResp.Data {
		task.Images = append(task.Images, image.ImageResult{
			Index:         i,
			URL:           d.URL,
			B64JSON:       d.B64JSON,
			RevisedPrompt: d.RevisedPrompt,
		})
	}
	return task, nil
}

func (e *NanoBananaExecutor) ImageToImage(_ *image.ImageToImageRequest) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *NanoBananaExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *NanoBananaExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("nanobanana: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

var reservedFields = map[string]bool{
	"model": true, "prompt": true, "n": true,
	"size": true, "quality": true, "response_format": true,
}

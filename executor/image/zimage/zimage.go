// Package zimage implements ImageExecutor for Z Image Turbo.
//
// API: https://api.zimage.ai/v1/images/generations (OpenAI-compatible)
// Model: z-image-turbo
// TextToImage only — ImageToImage returns ErrNotSupported.
package zimage

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
	image.RegisterImage("zimage", &ZImageExecutor{})
}

// ZImageExecutor handles Z Image Turbo generation (OpenAI-compatible API).
type ZImageExecutor struct {
	channel any
}

func (e *ZImageExecutor) Init(channel any) {
	e.channel = channel
}

func (e *ZImageExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "ZImage"
}

func (e *ZImageExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.zimage.ai"
}

func (e *ZImageExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *ZImageExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
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
	for k, v := range req.Extra {
		body[k] = v
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("zimage: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zimage: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zimage: HTTP %d: %s", resp.StatusCode, string(raw))
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
		return nil, fmt.Errorf("zimage: unmarshal: %w", err)
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

func (e *ZImageExecutor) ImageToImage(_ *image.ImageToImageRequest) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *ZImageExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *ZImageExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("zimage: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

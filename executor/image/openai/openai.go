// Package openai implements ImageExecutor for OpenAI GPT Image 2 (DALL-E 3/GPT Image).
//
// Endpoints:
//   - POST /v1/images/generations — TextToImage
//   - POST /v1/images/edits — ImageToImage (edit)
//
// Models: dall-e-3, gpt-image-2, gpt-image-2-pro
package openai

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
	image.RegisterImage("openai", &GPTImageExecutor{})
}

// GPTImageExecutor handles OpenAI GPT Image and DALL-E generation.
type GPTImageExecutor struct {
	channel any
}

func (e *GPTImageExecutor) Init(channel any) {
	e.channel = channel
}

func (e *GPTImageExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "GPTImage"
}

// openaiImageRequest maps to POST /v1/images/generations body.
type openaiImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"` // OpenAI-specific vivid/natural
}

// openaiImageResponse maps to POST /v1/images/generations response.
type openaiImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url,omitempty"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	} `json:"data"`
}

func (e *GPTImageExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.openai.com"
}

func (e *GPTImageExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *GPTImageExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	body := openaiImageRequest{
		Model:          req.Model,
		Prompt:         req.Prompt,
		N:              req.N,
		Size:           req.Size,
		Quality:        req.Quality,
		ResponseFormat: req.ResponseFormat,
	}

	// Map extra passthrough
	if req.Extra != nil {
		if style, ok := req.Extra["style"].(string); ok {
			body.Style = style
		}
	}

	// Defaults
	if body.N == 0 {
		body.N = 1
	}
	if body.ResponseFormat == "" {
		body.ResponseFormat = "url"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gptimage: marshal request: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gptimage: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gptimage: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("gptimage: unmarshal response: %w", err)
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

func (e *GPTImageExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	body := openaiImageRequest{
		Model:          req.Model,
		Prompt:         req.Prompt,
		N:              req.N,
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
	}

	if body.N == 0 {
		body.N = 1
	}
	if body.ResponseFormat == "" {
		body.ResponseFormat = "url"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gptimage: marshal request: %w", err)
	}

	// For GPT Image 2 / DALL-E, edits go to /v1/images/edits
	resp, err := e.doRequest("/v1/images/edits", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gptimage: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gptimage: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("gptimage: unmarshal response: %w", err)
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

func (e *GPTImageExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *GPTImageExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	reqURL := baseURL + path

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gptimage: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	return client.Do(req)
}

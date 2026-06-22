// Package zhipu implements ImageExecutor for Zhipu CogView image generation.
//
// Endpoint:
//   - POST /v1/images/generations — TextToImage, ImageToImage
//
// Models: cogview-4, cogview-3-plus, cogview-3-flash, cogview-3
// cogview-3-plus supports image_reference via Extra map passthrough.
package zhipu

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
	image.RegisterImage("zhipu", &CogViewExecutor{})
}

// CogViewExecutor handles Zhipu CogView image generation.
type CogViewExecutor struct {
	channel any
}

func (e *CogViewExecutor) Init(channel any) {
	e.channel = channel
}

func (e *CogViewExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "CogView"
}

// openaiImageRequest maps to POST /v1/images/generations body.
type openaiImageRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
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

func (e *CogViewExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://open.bigmodel.cn/api/paas/v4"
}

func (e *CogViewExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *CogViewExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
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
		return nil, fmt.Errorf("cogview: marshal request: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cogview: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cogview: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("cogview: unmarshal response: %w", err)
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

func (e *CogViewExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	// Build request as a map to allow merging extra fields (image_reference etc.)
	m := map[string]any{
		"prompt": req.Prompt,
	}
	if req.Model != "" {
		m["model"] = req.Model
	}
	if req.N > 0 {
		m["n"] = req.N
	}
	if req.Size != "" {
		m["size"] = req.Size
	}
	if req.ResponseFormat != "" {
		m["response_format"] = req.ResponseFormat
	}

	// Merge extra passthrough (e.g. image_reference for cogview-3-plus)
	for k, v := range req.Extra {
		m[k] = v
	}

	// Defaults
	if _, ok := m["n"]; !ok {
		m["n"] = 1
	}
	if _, ok := m["response_format"]; !ok {
		m["response_format"] = "url"
	}

	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("cogview: marshal request: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cogview: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cogview: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("cogview: unmarshal response: %w", err)
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

func (e *CogViewExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *CogViewExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	reqURL := baseURL + path

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("cogview: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	return client.Do(req)
}

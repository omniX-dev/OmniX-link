// Package stepfun implements ImageExecutor for Stepfun (阶跃星辰) image generation/editing.
//
// Endpoints:
//   - POST /v1/images/generations — TextToImage
//   - POST /v1/images/edits — ImageToImage (edit)
//
// Models: step-image-edit-2
package stepfun

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
	image.RegisterImage("stepfun", &StepImageExecutor{})
}

// StepImageExecutor handles Stepfun image generation and editing.
type StepImageExecutor struct {
	channel any
}

func (e *StepImageExecutor) Init(channel any) {
	e.channel = channel
}

func (e *StepImageExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "StepImage"
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

func (e *StepImageExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.stepfun.com/v1"
}

func (e *StepImageExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *StepImageExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
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
		return nil, fmt.Errorf("stepimage: marshal request: %w", err)
	}

	resp, err := e.doRequest("/v1/images/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stepimage: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stepimage: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("stepimage: unmarshal response: %w", err)
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

func (e *StepImageExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
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
		return nil, fmt.Errorf("stepimage: marshal request: %w", err)
	}

	resp, err := e.doRequest("/v1/images/edits", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stepimage: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stepimage: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var oaiResp openaiImageResponse
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("stepimage: unmarshal response: %w", err)
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

func (e *StepImageExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

func (e *StepImageExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	reqURL := baseURL + path

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("stepimage: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	return client.Do(req)
}

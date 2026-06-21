// Package qwen implements ImageExecutor for Alibaba Qwen Image.
//
// API: DashScope — POST /api/v1/services/aigc/text2image/image-synthesis
// Models: qwen-max, qwen-plus, qwen-turbo (image generation variants)
// Supports T2I and I2I (inpainting/outpainting).
package qwen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor/image"
)

func init() {
	image.RegisterImage("qwen", &QwenImageExecutor{})
}

// QwenImageExecutor handles Alibaba Cloud Qwen Image generation via DashScope API.
type QwenImageExecutor struct {
	channel any
}

func (e *QwenImageExecutor) Init(channel any) {
	e.channel = channel
}

func (e *QwenImageExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Qwen"
}

// dashscopeT2IRequest maps to DashScope text2image API.
type dashscopeT2IRequest struct {
	Model    string `json:"model"`
	Input    struct {
		Prompt string `json:"prompt"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// dashscopeT2IResponse maps to DashScope text2image response.
type dashscopeT2IResponse struct {
	Output struct {
		TaskID      string `json:"task_id,omitempty"`
		TaskStatus  string `json:"task_status,omitempty"`
		Code        string `json:"code,omitempty"`
		Message     string `json:"message,omitempty"`
		Results     []struct {
			URL     string `json:"url"`
			Seed    int64  `json:"seed,omitempty"`
		} `json:"results,omitempty"`
	} `json:"output"`
}

func (e *QwenImageExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *QwenImageExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *QwenImageExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "qwen-max"
	}

	dashReq := dashscopeT2IRequest{
		Model: model,
	}
	dashReq.Input.Prompt = req.Prompt

	// Map standard params to DashScope parameters
	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.N > 1 {
		params["n"] = req.N
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	if req.ResponseFormat == "b64_json" {
		params["response_format"] = "b64_json"
	}
	// Extra passthrough
	for k, v := range req.Extra {
		params[k] = v
	}
	dashReq.Parameters = params

	payload, err := json.Marshal(dashReq)
	if err != nil {
		return nil, fmt.Errorf("qwen: marshal request: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/aigc/text2image/image-synthesis", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qwen: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dashResp dashscopeT2IResponse
	if err := json.Unmarshal(raw, &dashResp); err != nil {
		return nil, fmt.Errorf("qwen: unmarshal response: %w", err)
	}

	if dashResp.Output.Code != "" {
		return nil, fmt.Errorf("qwen: API error: %s: %s", dashResp.Output.Code, dashResp.Output.Message)
	}

	// DashScope image synthesis is async — returns pending task, need to poll
	task := &image.ImageTask{
		ID:        dashResp.Output.TaskID,
		Status:    image.TaskStatusPending,
		CreatedAt: time.Now().Unix(),
	}

	// If task already completed
	if dashResp.Output.TaskStatus == "SUCCESS" || dashResp.Output.TaskStatus == "SUCCEEDED" {
		task.Status = image.TaskStatusCompleted
		for _, r := range dashResp.Output.Results {
			task.Images = append(task.Images, image.ImageResult{
				URL:  r.URL,
				Seed: r.Seed,
			})
		}
	}

	return task, nil
}

func (e *QwenImageExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "qwen-max"
	}

	dashReq := dashscopeT2IRequest{
		Model: model,
	}
	dashReq.Input.Prompt = req.Prompt

	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Image != "" {
		params["image"] = req.Image
	}
	if req.Strength > 0 {
		params["strength"] = req.Strength
	}
	if req.ResponseFormat == "b64_json" {
		params["response_format"] = "b64_json"
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	dashReq.Parameters = params

	payload, err := json.Marshal(dashReq)
	if err != nil {
		return nil, fmt.Errorf("qwen: marshal request: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/aigc/text2image/image-synthesis", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qwen: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dashResp dashscopeT2IResponse
	if err := json.Unmarshal(raw, &dashResp); err != nil {
		return nil, fmt.Errorf("qwen: unmarshal response: %w", err)
	}

	if dashResp.Output.Code != "" {
		return nil, fmt.Errorf("qwen: API error: %s: %s", dashResp.Output.Code, dashResp.Output.Message)
	}

	task := &image.ImageTask{
		ID:        dashResp.Output.TaskID,
		Status:    image.TaskStatusPending,
		CreatedAt: time.Now().Unix(),
	}

	if dashResp.Output.TaskStatus == "SUCCESS" || dashResp.Output.TaskStatus == "SUCCEEDED" {
		task.Status = image.TaskStatusCompleted
		for _, r := range dashResp.Output.Results {
			task.Images = append(task.Images, image.ImageResult{
				URL:  r.URL,
				Seed: r.Seed,
			})
		}
	}

	return task, nil
}

func (e *QwenImageExecutor) GetTask(taskID string) (*image.ImageTask, error) {
	// DashScope task query endpoint
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") +
		"/api/v1/tasks/" + taskID

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("qwen: create task request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qwen: query task: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("qwen: read task response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qwen: task HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dashResp dashscopeT2IResponse
	if err := json.Unmarshal(raw, &dashResp); err != nil {
		return nil, fmt.Errorf("qwen: unmarshal task response: %w", err)
	}

	task := &image.ImageTask{
		ID:     taskID,
		Status: image.TaskStatusPending,
	}

	switch dashResp.Output.TaskStatus {
	case "SUCCESS", "SUCCEEDED":
		task.Status = image.TaskStatusCompleted
		for _, r := range dashResp.Output.Results {
			task.Images = append(task.Images, image.ImageResult{
				URL:  r.URL,
				Seed: r.Seed,
			})
		}
	case "FAILED":
		task.Status = image.TaskStatusFailed
		task.Error = dashResp.Output.Message
	case "RUNNING", "PENDING":
		task.Status = image.TaskStatusProcessing
	}

	return task, nil
}

func (e *QwenImageExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	reqURL := baseURL + path

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("qwen: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	return client.Do(req)
}

// Package alibaba implements ImageExecutor for Alibaba Wan2.5 T2I and I2I.
//
// API: DashScope — POST /api/v1/services/aigc/text2image/image-synthesis
// Models: wan2.5-t2i, wan2.5-i2i
// Supports: TextToImage (wan2.5-t2i), ImageToImage (wan2.5-i2i)
package alibaba

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
	image.RegisterImage("wan", &WanImageExecutor{})
}

// WanImageExecutor handles Alibaba Wan2.5 image generation via DashScope.
type WanImageExecutor struct {
	channel any
}

func (e *WanImageExecutor) Init(channel any) {
	e.channel = channel
}

func (e *WanImageExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Wan"
}

type dashScopeReq struct {
	Model      string         `json:"model"`
	Input      map[string]any `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type dashScopeResp struct {
	Output struct {
		TaskID     string `json:"task_id,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Results    []struct {
			URL  string `json:"url"`
			Seed int64  `json:"seed,omitempty"`
		} `json:"results,omitempty"`
	} `json:"output"`
}

func (e *WanImageExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *WanImageExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *WanImageExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "wan2.5-t2i"
	}

	dReq := dashScopeReq{
		Model: model,
		Input: map[string]any{"prompt": req.Prompt},
	}
	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Quality != "" {
		params["quality"] = req.Quality
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	dReq.Parameters = params

	return e.submitAndPoll(dReq, model)
}

func (e *WanImageExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "wan2.5-i2i"
	}

	input := map[string]any{"prompt": req.Prompt}
	if req.Image != "" {
		input["image"] = req.Image
	}
	if req.Mask != "" {
		input["mask"] = req.Mask
	}

	dReq := dashScopeReq{
		Model: model,
		Input: input,
	}
	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Strength > 0 {
		params["strength"] = req.Strength
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	dReq.Parameters = params

	return e.submitAndPoll(dReq, model)
}

func (e *WanImageExecutor) GetTask(taskID string) (*image.ImageTask, error) {
	return e.queryTask(taskID)
}

func (e *WanImageExecutor) submitAndPoll(dReq dashScopeReq, _ string) (*image.ImageTask, error) {
	payload, err := json.Marshal(dReq)
	if err != nil {
		return nil, fmt.Errorf("wan: marshal: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/aigc/text2image/image-synthesis", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wan: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wan: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dResp dashScopeResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("wan: unmarshal: %w", err)
	}
	if dResp.Output.Code != "" {
		return nil, fmt.Errorf("wan: %s: %s", dResp.Output.Code, dResp.Output.Message)
	}

	task := &image.ImageTask{
		ID:        dResp.Output.TaskID,
		Status:    image.TaskStatusPending,
		CreatedAt: time.Now().Unix(),
	}

	// Some DashScope calls return sync results
	if dResp.Output.TaskStatus == "SUCCESS" || dResp.Output.TaskStatus == "SUCCEEDED" {
		task.Status = image.TaskStatusCompleted
		for _, r := range dResp.Output.Results {
			task.Images = append(task.Images, image.ImageResult{
				URL:  r.URL,
				Seed: r.Seed,
			})
		}
	}

	return task, nil
}

func (e *WanImageExecutor) queryTask(taskID string) (*image.ImageTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/api/v1/tasks/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wan: create task req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("wan: query task: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wan: read task: %w", err)
	}

	var dResp dashScopeResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("wan: unmarshal task: %w", err)
	}

	task := &image.ImageTask{ID: taskID}
	switch dResp.Output.TaskStatus {
	case "SUCCESS", "SUCCEEDED":
		task.Status = image.TaskStatusCompleted
		for _, r := range dResp.Output.Results {
			task.Images = append(task.Images, image.ImageResult{URL: r.URL, Seed: r.Seed})
		}
	case "FAILED":
		task.Status = image.TaskStatusFailed
		task.Error = dResp.Output.Message
	default:
		task.Status = image.TaskStatusProcessing
	}
	return task, nil
}

func (e *WanImageExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("wan: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

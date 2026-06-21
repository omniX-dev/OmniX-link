// Package midjourney implements ImageExecutor for Midjourney API.
//
// Midjourney uses an async pattern: POST /v1/imagine → task ID → GET /v1/task/{id}/fetch
// All T2I and I2I calls return a pending task — client must poll GetTask.
package midjourney

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
	image.RegisterImage("midjourney", &MidjourneyExecutor{})
}

// MidjourneyExecutor handles Midjourney image generation via custom REST API.
type MidjourneyExecutor struct {
	channel any
}

func (e *MidjourneyExecutor) Init(channel any) {
	e.channel = channel
}

func (e *MidjourneyExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Midjourney"
}

// mjImagineRequest maps to Midjourney /v1/imagine.
type mjImagineRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
	Ratio  string `json:"ratio,omitempty"`
	Style  string `json:"style,omitempty"`
	No     string `json:"no,omitempty"`     // negative prompt
	Seed   int64  `json:"seed,omitempty"`
	Chaos  int    `json:"chaos,omitempty"`
	Stile  int    `json:"stile,omitempty"`
}

type mjImagineResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  string `json:"result"` // task ID
}

type mjFetchResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  struct {
		ID       string `json:"id"`
		Status   string `json:"status"` // "pending", "processing", "completed", "failed"
		Action   string `json:"action"`
		Prompt   string `json:"prompt"`
		ImageURL string `json:"imageUrl,omitempty"`
		Image    string `json:"image,omitempty"`
		Progress string `json:"progress,omitempty"`
		Fail     string `json:"fail,omitempty"`
	} `json:"result"`
}

func (e *MidjourneyExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.midjourney.ai"
}

func (e *MidjourneyExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *MidjourneyExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	mjReq := mjImagineRequest{
		Prompt: req.Prompt,
	}
	if req.Model != "" {
		mjReq.Model = req.Model
	}
	if req.Extra != nil {
		if v, ok := req.Extra["ratio"].(string); ok {
			mjReq.Ratio = v
		}
		if v, ok := req.Extra["style"].(string); ok {
			mjReq.Style = v
		}
		if v, ok := req.Extra["no"].(string); ok {
			mjReq.No = v
		}
		if v, ok := req.Extra["chaos"].(float64); ok {
			mjReq.Chaos = int(v)
		}
	}
	// Midjourney default ratio
	if mjReq.Ratio == "" {
		mjReq.Ratio = "1:1"
	}

	payload, err := json.Marshal(mjReq)
	if err != nil {
		return nil, fmt.Errorf("midjourney: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/imagine", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midjourney: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("midjourney: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var mjResp mjImagineResponse
	if err := json.Unmarshal(raw, &mjResp); err != nil {
		return nil, fmt.Errorf("midjourney: unmarshal: %w", err)
	}
	if mjResp.Code != 1 && mjResp.Result == "" {
		return nil, fmt.Errorf("midjourney: %s", mjResp.Message)
	}

	return &image.ImageTask{
		ID:        mjResp.Result,
		Status:    image.TaskStatusPending,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func (e *MidjourneyExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	// Midjourney I2I uses /v1/imagine with image URL in prompt
	prompt := req.Prompt
	if req.Image != "" {
		if prompt != "" {
			prompt = req.Image + " " + prompt
		} else {
			prompt = req.Image
		}
	}

	mjReq := mjImagineRequest{
		Prompt: prompt,
	}
	if req.Extra != nil {
		if v, ok := req.Extra["ratio"].(string); ok {
			mjReq.Ratio = v
		}
		if v, ok := req.Extra["style"].(string); ok {
			mjReq.Style = v
		}
	}
	if mjReq.Ratio == "" {
		mjReq.Ratio = "1:1"
	}

	payload, err := json.Marshal(mjReq)
	if err != nil {
		return nil, fmt.Errorf("midjourney: marshal i2i: %w", err)
	}

	resp, err := e.doRequest("/v1/imagine", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midjourney: read i2i: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("midjourney: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var mjResp mjImagineResponse
	if err := json.Unmarshal(raw, &mjResp); err != nil {
		return nil, fmt.Errorf("midjourney: unmarshal i2i: %w", err)
	}
	if mjResp.Code != 1 && mjResp.Result == "" {
		return nil, fmt.Errorf("midjourney: %s", mjResp.Message)
	}

	return &image.ImageTask{
		ID:        mjResp.Result,
		Status:    image.TaskStatusPending,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func (e *MidjourneyExecutor) GetTask(taskID string) (*image.ImageTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/v1/task/" + taskID + "/fetch"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("midjourney: create fetch req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("midjourney: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("midjourney: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("midjourney: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var mjResp mjFetchResponse
	if err := json.Unmarshal(raw, &mjResp); err != nil {
		return nil, fmt.Errorf("midjourney: unmarshal fetch: %w", err)
	}

	task := &image.ImageTask{ID: taskID}
	switch mjResp.Result.Status {
	case "completed":
		task.Status = image.TaskStatusCompleted
		imgURL := mjResp.Result.ImageURL
		if imgURL == "" {
			imgURL = mjResp.Result.Image
		}
		if imgURL != "" {
			task.Images = append(task.Images, image.ImageResult{URL: imgURL})
		}
	case "failed":
		task.Status = image.TaskStatusFailed
		task.Error = mjResp.Result.Fail
		if task.Error == "" {
			task.Error = mjResp.Message
		}
	case "processing":
		task.Status = image.TaskStatusProcessing
	default:
		task.Status = image.TaskStatusPending
	}

	return task, nil
}

func (e *MidjourneyExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("midjourney: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

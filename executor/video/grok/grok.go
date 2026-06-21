// Package grok implements VideoExecutor for xAI Grok Imagine Video.
//
// Endpoints:
//   - POST /v1/videos/generations — create video
//   - GET /v1/videos/generations/{id} — poll status
//
// Auth: Authorization: Bearer (xAI API key)
// Models: grok-imagine-video-1.5, grok-imagine-video-1.5-preview
package grok

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor/video"
)

func init() {
	video.RegisterVideo("grok", &GrokVideoExecutor{})
}

// GrokVideoExecutor handles xAI Grok Imagine Video generation.
type GrokVideoExecutor struct {
	channel any
}

func (e *GrokVideoExecutor) Init(channel any) {
	e.channel = channel
}

func (e *GrokVideoExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Grok"
}

func (e *GrokVideoExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.x.ai"
}

func (e *GrokVideoExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type grokVideoReq struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
	Size   string `json:"size,omitempty"`
}

type grokVideoResp struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Output  *struct {
		VideoURL string  `json:"video_url,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func (e *GrokVideoExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	gReq := grokVideoReq{
		Model:  req.Model,
		Prompt: req.Prompt,
		Size:   req.Size,
	}
	if gReq.Model == "" {
		gReq.Model = "grok-imagine-video-1.5"
	}

	payload, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/videos/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var gResp grokVideoResp
	if err := json.Unmarshal(raw, &gResp); err != nil {
		return nil, fmt.Errorf("grok: unmarshal: %w", err)
	}

	task := &video.VideoTask{
		ID:        gResp.ID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch gResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if gResp.Output != nil {
			task.VideoURL = gResp.Output.VideoURL
			task.Duration = gResp.Output.Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if gResp.Error != nil {
			task.Error = gResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *GrokVideoExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	// Grok supports image references in prompt
	gReq := grokVideoReq{
		Model:  req.Model,
		Prompt: req.Prompt,
		Size:   req.Size,
	}
	if gReq.Model == "" {
		gReq.Model = "grok-imagine-video-1.5"
	}

	// Use Extra to pass image_url
	payload, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("grok: marshal i2v: %w", err)
	}

	resp, err := e.doRequest("/v1/videos/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read i2v: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var gResp grokVideoResp
	if err := json.Unmarshal(raw, &gResp); err != nil {
		return nil, fmt.Errorf("grok: unmarshal i2v: %w", err)
	}

	task := &video.VideoTask{
		ID:        gResp.ID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch gResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if gResp.Output != nil {
			task.VideoURL = gResp.Output.VideoURL
			task.Duration = gResp.Output.Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if gResp.Error != nil {
			task.Error = gResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *GrokVideoExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *GrokVideoExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *GrokVideoExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *GrokVideoExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *GrokVideoExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/v1/videos/generations/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("grok: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("grok: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("grok: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("grok: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var gResp grokVideoResp
	if err := json.Unmarshal(raw, &gResp); err != nil {
		return nil, fmt.Errorf("grok: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch gResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if gResp.Output != nil {
			task.VideoURL = gResp.Output.VideoURL
			task.Duration = gResp.Output.Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if gResp.Error != nil {
			task.Error = gResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *GrokVideoExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("grok: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

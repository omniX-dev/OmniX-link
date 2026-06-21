// Package runway implements VideoExecutor for Runway Gen-4 video generation.
//
// Endpoints:
//   - POST /v1/text_to_video — T2V
//   - POST /v1/image_to_video — I2V
//   - GET /v1/tasks/{id} — poll status
//
// Auth: Authorization: Bearer + X-Runway-Version
// Models: gen4.5, gen4_turbo, gen4_aleph
// Act Two character performance.
package runway

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
	video.RegisterVideo("runway", &RunwayExecutor{})
}

// RunwayExecutor handles Runway Gen-4 video generation.
type RunwayExecutor struct {
	channel any
}

func (e *RunwayExecutor) Init(channel any) {
	e.channel = channel
}

func (e *RunwayExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Runway"
}

func (e *RunwayExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.dev.runwayml.com"
}

func (e *RunwayExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type runwayTaskReq struct {
	Model        string `json:"model,omitempty"`
	PromptText   string `json:"prompt_text,omitempty"`
	PromptImage  string `json:"prompt_image,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
}

type runwayTaskResp struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output *struct {
		VideoURL     string `json:"video_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func (e *RunwayExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	rReq := runwayTaskReq{
		Model:      req.Model,
		PromptText: req.Prompt,
		Duration:   req.Duration,
		Resolution: req.Size,
	}
	if rReq.Model == "" {
		rReq.Model = "gen4_turbo"
	}

	payload, err := json.Marshal(rReq)
	if err != nil {
		return nil, fmt.Errorf("runway: marshal: %w", err)
	}

	return e.submitTask("/v1/text_to_video", payload)
}

func (e *RunwayExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	rReq := runwayTaskReq{
		Model:        req.Model,
		PromptText:   req.Prompt,
		PromptImage:  req.Image,
		Duration:     req.Duration,
		Resolution:   req.Size,
	}
	if rReq.Model == "" {
		rReq.Model = "gen4_turbo"
	}

	payload, err := json.Marshal(rReq)
	if err != nil {
		return nil, fmt.Errorf("runway: marshal i2v: %w", err)
	}

	return e.submitTask("/v1/image_to_video", payload)
}

func (e *RunwayExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *RunwayExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *RunwayExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *RunwayExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *RunwayExecutor) submitTask(path string, payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("runway: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runway: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var rResp runwayTaskResp
	if err := json.Unmarshal(raw, &rResp); err != nil {
		return nil, fmt.Errorf("runway: unmarshal: %w", err)
	}

	task := &video.VideoTask{
		ID:        rResp.ID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch rResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if rResp.Output != nil {
			task.VideoURL = rResp.Output.VideoURL
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if rResp.Error != nil {
			task.Error = rResp.Error.Message
		}
	case "processing", "running":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *RunwayExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/v1/tasks/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("runway: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("runway: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("runway: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runway: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var rResp runwayTaskResp
	if err := json.Unmarshal(raw, &rResp); err != nil {
		return nil, fmt.Errorf("runway: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch rResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if rResp.Output != nil {
			task.VideoURL = rResp.Output.VideoURL
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if rResp.Error != nil {
			task.Error = rResp.Error.Message
		}
	case "processing", "running":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *RunwayExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("runway: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	req.Header.Set("X-Runway-Version", "2025-03-13")
	return (&http.Client{}).Do(req)
}

// Package openai implements VideoExecutor for OpenAI Sora video generation.
//
// Endpoints:
//   - POST /v1/videos — create generation (T2V, I2V)
//   - GET /v1/videos/{id} — poll status
//
// ⚠️ OpenAI is discontinuing Sora 2 on September 24, 2026.
// Models: sora-2, sora-2-pro
package openai

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
	video.RegisterVideo("openai", &SoraExecutor{})
}

// SoraExecutor handles OpenAI Sora video generation.
type SoraExecutor struct {
	channel any
}

func (e *SoraExecutor) Init(channel any) {
	e.channel = channel
}

func (e *SoraExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Sora"
}

func (e *SoraExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.openai.com"
}

func (e *SoraExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// soraVideoRequest maps to OpenAI /v1/videos POST body.
type soraVideoRequest struct {
	Model    string         `json:"model,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Image    string         `json:"image,omitempty"`
	Duration int            `json:"duration,omitempty"`
	Size     string         `json:"size,omitempty"`
	Quality  string         `json:"quality,omitempty"`
	N        int            `json:"n,omitempty"`
	Extra    map[string]any `json:"-"`
}

type soraVideoResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output *struct {
		VideoURL     string  `json:"video_url,omitempty"`
		ThumbnailURL string  `json:"thumbnail_url,omitempty"`
		Duration     float64 `json:"duration,omitempty"`
		Size         string  `json:"size,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
	CreatedAt int64 `json:"created_at,omitempty"`
}

func (e *SoraExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	body := soraVideoRequest{
		Model:    req.Model,
		Prompt:   req.Prompt,
		Duration: req.Duration,
		Size:     req.Size,
		Quality:  req.Quality,
	}
	if body.Model == "" {
		body.Model = "sora-2"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("sora: marshal: %w", err)
	}

	return e.submitTask(payload)
}

func (e *SoraExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	body := soraVideoRequest{
		Model:    req.Model,
		Prompt:   req.Prompt,
		Image:    req.Image,
		Duration: req.Duration,
		Size:     req.Size,
	}
	if body.Model == "" {
		body.Model = "sora-2"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("sora: marshal i2v: %w", err)
	}

	return e.submitTask(payload)
}

func (e *SoraExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SoraExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SoraExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SoraExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *SoraExecutor) submitTask(payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest("/v1/videos", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sora: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sora: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var sResp soraVideoResponse
	if err := json.Unmarshal(raw, &sResp); err != nil {
		return nil, fmt.Errorf("sora: unmarshal: %w", err)
	}

	createdAt := sResp.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	task := &video.VideoTask{
		ID:        sResp.ID,
		Status:    video.VideoTaskQueued,
		CreatedAt: createdAt,
	}

	switch sResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if sResp.Output != nil {
			task.VideoURL = sResp.Output.VideoURL
			task.ThumbnailURL = sResp.Output.ThumbnailURL
			task.Duration = sResp.Output.Duration
			task.Size = sResp.Output.Size
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if sResp.Error != nil {
			task.Error = sResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *SoraExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/v1/videos/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("sora: create fetch req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("sora: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("sora: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sora: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var sResp soraVideoResponse
	if err := json.Unmarshal(raw, &sResp); err != nil {
		return nil, fmt.Errorf("sora: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch sResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if sResp.Output != nil {
			task.VideoURL = sResp.Output.VideoURL
			task.ThumbnailURL = sResp.Output.ThumbnailURL
			task.Duration = sResp.Output.Duration
			task.Size = sResp.Output.Size
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if sResp.Error != nil {
			task.Error = sResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *SoraExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("sora: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

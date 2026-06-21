// Package seedance implements VideoExecutor for ByteDance Seedance (via fal.ai).
//
// Endpoints (via fal.ai):
//   - POST /fal-ai/bytedance/seedance-2.0/text-to-video — T2V
//   - POST /fal-ai/bytedance/seedance-2.0/image-to-video — I2V
//
// Features: 2K resolution, native audio, sync audio+video.
package seedance

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
	video.RegisterVideo("seedance", &SeedanceExecutor{})
}

// SeedanceExecutor handles ByteDance Seedance video via fal.ai.
type SeedanceExecutor struct {
	channel any
}

func (e *SeedanceExecutor) Init(channel any) {
	e.channel = channel
}

func (e *SeedanceExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Seedance"
}

func (e *SeedanceExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://fal.run"
}

func (e *SeedanceExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type falTaskResp struct {
	RequestID string `json:"request_id,omitempty"`
	Status    string `json:"status"`
	Output    *struct {
		VideoURL string  `json:"video_url,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func (e *SeedanceExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "seedance-2.0"
	}

	falReq := map[string]any{
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		falReq["image_size"] = req.Size
	}
	if req.Duration > 0 {
		falReq["duration"] = req.Duration
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("seedance: marshal: %w", err)
	}

	return e.submitTask("/fal-ai/bytedance/seedance-2.0/text-to-video", payload)
}

func (e *SeedanceExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	falReq := map[string]any{
		"prompt":    req.Prompt,
		"image_url": req.Image,
	}
	if req.Size != "" {
		falReq["image_size"] = req.Size
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("seedance: marshal i2v: %w", err)
	}

	return e.submitTask("/fal-ai/bytedance/seedance-2.0/image-to-video", payload)
}

func (e *SeedanceExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SeedanceExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SeedanceExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *SeedanceExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *SeedanceExecutor) submitTask(path string, payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("seedance: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seedance: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp falTaskResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("seedance: unmarshal: %w", err)
	}

	task := &video.VideoTask{
		ID:        falResp.RequestID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch falResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if falResp.Output != nil {
			task.VideoURL = falResp.Output.VideoURL
			task.Duration = falResp.Output.Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if falResp.Error != nil {
			task.Error = falResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *SeedanceExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/fal-ai/bytedance/seedance-2.0/requests/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("seedance: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Key "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("seedance: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("seedance: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seedance: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp falTaskResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("seedance: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch falResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if falResp.Output != nil {
			task.VideoURL = falResp.Output.VideoURL
			task.Duration = falResp.Output.Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if falResp.Error != nil {
			task.Error = falResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *SeedanceExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("seedance: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

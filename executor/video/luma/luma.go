// Package luma implements VideoExecutor for Luma AI Ray3.2 (via fal.ai).
//
// Endpoints (via fal.ai):
//   - POST /fal-ai/luma/ray-3.2/text-to-video — T2V
//   - POST /fal-ai/luma/ray-3.2/image-to-video — I2V
//
// Also available via Replicate and official API.
// Models: ray-3.2, ray-2, ray-flash-2
// Features: Frame-level keyframe control, HDR export.
package luma

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
	video.RegisterVideo("luma", &LumaExecutor{})
}

// LumaExecutor handles Luma AI video via fal.ai.
type LumaExecutor struct {
	channel any
}

func (e *LumaExecutor) Init(channel any) {
	e.channel = channel
}

func (e *LumaExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Luma"
}

func (e *LumaExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://fal.run"
}

func (e *LumaExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type lumaFalResp struct {
	RequestID string `json:"request_id,omitempty"`
	Status    string `json:"status"`
	Output    *struct {
		VideoURL string `json:"video_url,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func (e *LumaExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "ray-3.2"
	}

	falReq := map[string]any{
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		falReq["aspect_ratio"] = req.Size
	}
	if req.Duration > 0 {
		falReq["duration"] = req.Duration
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("luma: marshal: %w", err)
	}

	return e.submitTask("/fal-ai/luma/ray-3.2/text-to-video", payload)
}

func (e *LumaExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	falReq := map[string]any{
		"prompt":    req.Prompt,
		"image_url": req.Image,
	}
	if req.Size != "" {
		falReq["aspect_ratio"] = req.Size
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("luma: marshal i2v: %w", err)
	}

	return e.submitTask("/fal-ai/luma/ray-3.2/image-to-video", payload)
}

func (e *LumaExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *LumaExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *LumaExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *LumaExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *LumaExecutor) submitTask(path string, payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("luma: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("luma: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp lumaFalResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("luma: unmarshal: %w", err)
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

func (e *LumaExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/fal-ai/luma/ray-3.2/requests/"+taskID+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("luma: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Key "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("luma: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("luma: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("luma: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp lumaFalResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("luma: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch falResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if falResp.Output != nil {
			task.VideoURL = falResp.Output.VideoURL
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

func (e *LumaExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("luma: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

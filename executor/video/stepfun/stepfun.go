// Package stepfun implements VideoExecutor for Stepfun (阶跃星辰) Step Video.
//
// Endpoints:
//   - POST /v1/video/generations — T2V + I2V
//   - GET  /v1/video/status/{taskID} — poll task status
//
// Auth: Authorization: Bearer
// Models: step-video-ti2v
package stepfun

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
	video.RegisterVideo("stepfun", &StepVideoExecutor{})
}

// StepVideoExecutor handles Stepfun Step Video generation.
type StepVideoExecutor struct {
	channel any
}

func (e *StepVideoExecutor) Init(channel any) {
	e.channel = channel
}

func (e *StepVideoExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "StepVideo"
}

func (e *StepVideoExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.stepfun.com/v1"
}

func (e *StepVideoExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// stepVideoRequest maps to Step Video generation request payload.
type stepVideoRequest struct {
	Model     string `json:"model,omitempty"`
	Prompt    string `json:"prompt"`
	Image     string `json:"image,omitempty"`
	VideoSize string `json:"video_size,omitempty"`
}

// stepVideoResponse maps to Step Video generation response.
type stepVideoResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	VideoURL string `json:"video_url,omitempty"`
}

func (e *StepVideoExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	vReq := stepVideoRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
	}
	if vReq.Model == "" {
		vReq.Model = "step-video-ti2v"
	}
	if req.Size != "" {
		vReq.VideoSize = req.Size
	} else {
		vReq.VideoSize = "960x540"
	}

	payload, err := json.Marshal(vReq)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: marshal: %w", err)
	}

	return e.submitTask(payload)
}

func (e *StepVideoExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	vReq := stepVideoRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Image:  req.Image,
	}
	if vReq.Model == "" {
		vReq.Model = "step-video-ti2v"
	}
	if req.Size != "" {
		vReq.VideoSize = req.Size
	} else {
		vReq.VideoSize = "960x540"
	}

	payload, err := json.Marshal(vReq)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: marshal i2v: %w", err)
	}

	return e.submitTask(payload)
}

func (e *StepVideoExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *StepVideoExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *StepVideoExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *StepVideoExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *StepVideoExecutor) submitTask(payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest("/v1/video/generations", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stepvideo: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var svResp stepVideoResponse
	if err := json.Unmarshal(raw, &svResp); err != nil {
		return nil, fmt.Errorf("stepvideo: unmarshal: %w", err)
	}

	task := &video.VideoTask{
		ID:        svResp.ID,
		CreatedAt: time.Now().Unix(),
	}

	switch svResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		task.VideoURL = svResp.VideoURL
	case "failed":
		task.Status = video.VideoTaskFailed
		task.Error = svResp.VideoURL
		if task.Error == "" {
			task.Error = "generation failed"
		}
	default:
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *StepVideoExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/v1/video/status/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stepvideo: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stepvideo: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var svResp stepVideoResponse
	if err := json.Unmarshal(raw, &svResp); err != nil {
		return nil, fmt.Errorf("stepvideo: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch svResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		task.VideoURL = svResp.VideoURL
	case "failed":
		task.Status = video.VideoTaskFailed
		task.Error = svResp.VideoURL
		if task.Error == "" {
			task.Error = "generation failed"
		}
	default:
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *StepVideoExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("stepvideo: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

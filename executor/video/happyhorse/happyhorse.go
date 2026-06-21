// Package happyhorse implements VideoExecutor for Alibaba HappyHorse via DashScope.
//
// API: Same DashScope infra as Wan:
//   - POST /api/v1/services/aigc/video-generation/video-synthesis
//   - GET /api/v1/tasks/{id} — poll status
//
// Models: happyhorse-1.0-t2v, happyhorse-1.0-i2v, happyhorse-1.0-r2v, happyhorse-1.0-video-edit
// 720P / 1080P, 3-15 seconds.
package happyhorse

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
	video.RegisterVideo("happyhorse", &HappyHorseExecutor{})
}

// HappyHorseExecutor handles Alibaba HappyHorse video via DashScope.
type HappyHorseExecutor struct {
	channel any
}

func (e *HappyHorseExecutor) Init(channel any) {
	e.channel = channel
}

func (e *HappyHorseExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "HappyHorse"
}

func (e *HappyHorseExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *HappyHorseExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type dsVideoReq struct {
	Model      string         `json:"model"`
	Input      map[string]any `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type dsVideoResp struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		VideoURL   string `json:"video_url,omitempty"`
	} `json:"output"`
}

func (e *HappyHorseExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "happyhorse-1.0-t2v"
	}

	dReq := dsVideoReq{
		Model: model,
		Input: map[string]any{"prompt": req.Prompt},
	}
	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Duration > 0 {
		params["duration"] = req.Duration
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	dReq.Parameters = params

	return e.submitTask(dReq)
}

func (e *HappyHorseExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "happyhorse-1.0-i2v"
	}

	input := map[string]any{"prompt": req.Prompt}
	if req.Image != "" {
		input["image"] = req.Image
	}

	dReq := dsVideoReq{
		Model: model,
		Input: input,
	}
	params := make(map[string]any)
	if req.Size != "" {
		params["size"] = req.Size
	}
	if req.Duration > 0 {
		params["duration"] = req.Duration
	}
	for k, v := range req.Extra {
		params[k] = v
	}
	dReq.Parameters = params

	return e.submitTask(dReq)
}

func (e *HappyHorseExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HappyHorseExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HappyHorseExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HappyHorseExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *HappyHorseExecutor) submitTask(dReq dsVideoReq) (*video.VideoTask, error) {
	payload, err := json.Marshal(dReq)
	if err != nil {
		return nil, fmt.Errorf("happyhorse: marshal: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/aigc/video-generation/video-synthesis", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("happyhorse: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("happyhorse: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dResp dsVideoResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("happyhorse: unmarshal: %w", err)
	}
	if dResp.Output.Code != "" {
		return nil, fmt.Errorf("happyhorse: %s: %s", dResp.Output.Code, dResp.Output.Message)
	}

	task := &video.VideoTask{
		ID:        dResp.Output.TaskID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch dResp.Output.TaskStatus {
	case "SUCCESS", "SUCCEEDED":
		task.Status = video.VideoTaskCompleted
		task.VideoURL = dResp.Output.VideoURL
	case "FAILED":
		task.Status = video.VideoTaskFailed
		task.Error = dResp.Output.Message
	case "RUNNING":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *HappyHorseExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("happyhorse: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("happyhorse: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("happyhorse: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("happyhorse: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dResp dsVideoResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("happyhorse: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch dResp.Output.TaskStatus {
	case "SUCCESS", "SUCCEEDED":
		task.Status = video.VideoTaskCompleted
		task.VideoURL = dResp.Output.VideoURL
	case "FAILED":
		task.Status = video.VideoTaskFailed
		task.Error = dResp.Output.Message
	case "RUNNING":
		task.Status = video.VideoTaskProcessing
	default:
		task.Status = video.VideoTaskQueued
	}

	return task, nil
}

func (e *HappyHorseExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("happyhorse: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

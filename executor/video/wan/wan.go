// Package wan implements VideoExecutor for Alibaba Wan video generation.
//
// API: DashScope — POST /api/v1/services/aigc/video-generation/video-synthesis
// Models: wan2.7-t2v, wan2.7-i2v, wan2.7-videoedit
// Same DashScope infra as HappyHorse — shared polling endpoint.
package wan

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
	video.RegisterVideo("wan", &WanVideoExecutor{})
}

// WanVideoExecutor handles Alibaba Wan video generation via DashScope.
type WanVideoExecutor struct {
	channel any
}

func (e *WanVideoExecutor) Init(channel any) {
	e.channel = channel
}

func (e *WanVideoExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Wan"
}

func (e *WanVideoExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *WanVideoExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type dashScopeVideoReq struct {
	Model      string         `json:"model"`
	Input      map[string]any `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type dashScopeVideoResp struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		VideoURL   string `json:"video_url,omitempty"`
	} `json:"output"`
}

func (e *WanVideoExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "wan2.7-t2v"
	}

	dReq := dashScopeVideoReq{
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

func (e *WanVideoExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	model := req.Model
	if model == "" {
		model = "wan2.7-i2v"
	}

	input := map[string]any{"prompt": req.Prompt}
	if req.Image != "" {
		input["image"] = req.Image
	}

	dReq := dashScopeVideoReq{
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

func (e *WanVideoExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *WanVideoExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *WanVideoExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *WanVideoExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *WanVideoExecutor) submitTask(dReq dashScopeVideoReq) (*video.VideoTask, error) {
	payload, err := json.Marshal(dReq)
	if err != nil {
		return nil, fmt.Errorf("wan-video: marshal: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/aigc/video-generation/video-synthesis", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wan-video: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wan-video: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dResp dashScopeVideoResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("wan-video: unmarshal: %w", err)
	}
	if dResp.Output.Code != "" {
		return nil, fmt.Errorf("wan-video: %s: %s", dResp.Output.Code, dResp.Output.Message)
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

func (e *WanVideoExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/api/v1/tasks/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("wan-video: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("wan-video: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wan-video: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wan-video: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var dResp dashScopeVideoResp
	if err := json.Unmarshal(raw, &dResp); err != nil {
		return nil, fmt.Errorf("wan-video: unmarshal fetch: %w", err)
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

func (e *WanVideoExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("wan-video: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

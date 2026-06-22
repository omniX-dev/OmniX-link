// Package kuaishou implements VideoExecutor for Kuaishou Kling video generation.
//
// Endpoints:
//   - POST /v1/videos/text2video — T2V
//   - POST /v1/videos/image2video — I2V
//   - GET /v1/videos/{type}/{id} — poll status
//
// Auth: JWT (AK/SK signed, 30-min expiry)
// Models: kling-v3, kling-v2.6, kling-video-o1
package kuaishou

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/video"
)

func init() {
	video.RegisterVideo("kuaishou", &KlingExecutor{})
}

// KlingExecutor handles Kuaishou Kling video generation.
type KlingExecutor struct {
	channel any
}

func (e *KlingExecutor) Init(channel any) {
	e.channel = channel
}

func (e *KlingExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Kling"
}

func (e *KlingExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.klingai.com"
}

func (e *KlingExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// klingRequest maps to Kling video generation request.
type klingRequest struct {
	Model     string         `json:"model,omitempty"`
	Prompt    string         `json:"prompt"`
	Image     string         `json:"image,omitempty"`
	Size      string         `json:"size,omitempty"`
	Duration  int            `json:"duration,omitempty"`
	Negative  string         `json:"negative_prompt,omitempty"`
	N         int            `json:"n,omitempty"`
	Extra     map[string]any `json:"-"`
}

type klingResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		CreatedAt  int64  `json:"created_at,omitempty"`
	} `json:"data"`
}

type klingTaskResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID      string `json:"task_id"`
		TaskStatus  string `json:"task_status"`
		TaskStatusMsg string `json:"task_status_msg,omitempty"`
		Videos      []struct {
			ID       string `json:"id"`
			URL      string `json:"url"`
			Duration float64 `json:"duration,omitempty"`
		} `json:"videos,omitempty"`
	} `json:"data"`
}

func (e *KlingExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	kReq := klingRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		Size:    req.Size,
		Duration: req.Duration,
	}
	if kReq.Model == "" {
		kReq.Model = "kling-v3"
	}

	payload, err := json.Marshal(kReq)
	if err != nil {
		return nil, fmt.Errorf("kling: marshal: %w", err)
	}

	return e.submitTask("/v1/videos/text2video", payload)
}

func (e *KlingExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	kReq := klingRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		Image:   req.Image,
		Size:    req.Size,
		Duration: req.Duration,
	}
	if kReq.Model == "" {
		kReq.Model = "kling-v3"
	}

	payload, err := json.Marshal(kReq)
	if err != nil {
		return nil, fmt.Errorf("kling: marshal i2v: %w", err)
	}

	return e.submitTask("/v1/videos/image2video", payload)
}

func (e *KlingExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *KlingExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *KlingExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *KlingExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *KlingExecutor) submitTask(path string, payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kling: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kling: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var kResp klingResponse
	if err := json.Unmarshal(raw, &kResp); err != nil {
		return nil, fmt.Errorf("kling: unmarshal: %w", err)
	}
	if kResp.Code != 0 {
		return nil, fmt.Errorf("kling: %s (code %d)", kResp.Message, kResp.Code)
	}

	return &video.VideoTask{
		ID:        kResp.Data.TaskID,
		Status:    video.VideoTaskQueued,
		CreatedAt: kResp.Data.CreatedAt,
	}, nil
}

func (e *KlingExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	// Kling uses /v1/videos/text2video/{id} for task query
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/v1/videos/text2video/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("kling: create fetch: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sk, ok := e.getAPIKeyPair(); ok {
		// Kling uses JWT — set appropriate header
		req.Header.Set("Authorization", "Bearer "+sk)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("kling: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("kling: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kling: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var tResp klingTaskResponse
	if err := json.Unmarshal(raw, &tResp); err != nil {
		return nil, fmt.Errorf("kling: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch tResp.Data.TaskStatus {
	case "succeed", "completed":
		task.Status = video.VideoTaskCompleted
		if len(tResp.Data.Videos) > 0 {
			task.VideoURL = tResp.Data.Videos[0].URL
			task.Duration = tResp.Data.Videos[0].Duration
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		task.Error = tResp.Data.TaskStatusMsg
		if task.Error == "" {
			task.Error = tResp.Message
		}
	case "running", "processing":
		task.Status = video.VideoTaskProcessing
	default:
		task.Status = video.VideoTaskQueued
	}

	return task, nil
}

func (e *KlingExecutor) getAPIKeyPair() (string, bool) {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		key := ch.GetAPIKey()
		return key, key != ""
	}
	return "", false
}

func (e *KlingExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("kling: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Kling uses JWT auth
	if sk, ok := e.getAPIKeyPair(); ok {
		req.Header.Set("Authorization", "Bearer "+sk)
	}

	return (&http.Client{}).Do(req)
}

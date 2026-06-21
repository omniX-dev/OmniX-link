// Package hailuo implements VideoExecutor for MiniMax Hailuo video generation.
//
// Access via aggregators (JD Cloud, UCloud ModelVerse, Atlas Cloud).
// Custom REST endpoints vary by aggregator.
//
// Models: MiniMax-Hailuo-2.3, MiniMax-Hailuo-02
// Features: Camera movement directives: [推进], [拉远], [左移], [跟随]
package hailuo

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
	video.RegisterVideo("hailuo", &HailuoExecutor{})
}

// HailuoExecutor handles MiniMax Hailuo video via aggregator APIs.
type HailuoExecutor struct {
	channel any
}

func (e *HailuoExecutor) Init(channel any) {
	e.channel = channel
}

func (e *HailuoExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Hailuo"
}

func (e *HailuoExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.minimax.chat"
}

func (e *HailuoExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type hailuoReq struct {
	Model    string         `json:"model,omitempty"`
	Prompt   string         `json:"prompt"`
	ImageURL string         `json:"image_url,omitempty"`
	Duration int            `json:"duration,omitempty"`
	Size     string         `json:"size,omitempty"`
}

type hailuoResp struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg,omitempty"`
	} `json:"base_resp"`
	TaskID string `json:"task_id,omitempty"`
}

type hailuoTaskResp struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg,omitempty"`
	} `json:"base_resp"`
	Status string `json:"status"`
	FileID string `json:"file_id,omitempty"`
	Data   *struct {
		VideoURL string `json:"video_url,omitempty"`
	} `json:"data,omitempty"`
}

func (e *HailuoExecutor) TextToVideo(req *video.TextToVideoRequest) (*video.VideoTask, error) {
	hReq := hailuoReq{
		Model:    req.Model,
		Prompt:   req.Prompt,
		Duration: req.Duration,
		Size:     req.Size,
	}
	if hReq.Model == "" {
		hReq.Model = "MiniMax-Hailuo-2.3"
	}

	payload, err := json.Marshal(hReq)
	if err != nil {
		return nil, fmt.Errorf("hailuo: marshal: %w", err)
	}

	return e.submitTask("/v1/video_generation", payload)
}

func (e *HailuoExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	hReq := hailuoReq{
		Model:    req.Model,
		Prompt:   req.Prompt,
		ImageURL: req.Image,
		Duration: req.Duration,
	}
	if hReq.Model == "" {
		hReq.Model = "MiniMax-Hailuo-2.3"
	}

	payload, err := json.Marshal(hReq)
	if err != nil {
		return nil, fmt.Errorf("hailuo: marshal i2v: %w", err)
	}

	return e.submitTask("/v1/video_generation", payload)
}

func (e *HailuoExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HailuoExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HailuoExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *HailuoExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *HailuoExecutor) submitTask(path string, payload []byte) (*video.VideoTask, error) {
	resp, err := e.doRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hailuo: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hailuo: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var hResp hailuoResp
	if err := json.Unmarshal(raw, &hResp); err != nil {
		return nil, fmt.Errorf("hailuo: unmarshal: %w", err)
	}
	if hResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("hailuo: %s", hResp.BaseResp.StatusMsg)
	}

	return &video.VideoTask{
		ID:        hResp.TaskID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func (e *HailuoExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	body := map[string]string{"task_id": taskID}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", baseURL+"/v1/video_generation/query", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("hailuo: create query: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("hailuo: query: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hailuo: read query: %w", err)
	}

	var tResp hailuoTaskResp
	if err := json.Unmarshal(raw, &tResp); err != nil {
		return nil, fmt.Errorf("hailuo: unmarshal query: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch tResp.Status {
	case "success", "completed":
		task.Status = video.VideoTaskCompleted
		if tResp.Data != nil {
			task.VideoURL = tResp.Data.VideoURL
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		task.Error = tResp.BaseResp.StatusMsg
	case "processing", "running":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *HailuoExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("hailuo: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

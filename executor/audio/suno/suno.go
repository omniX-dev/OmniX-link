// Package suno implements AudioExecutor for Suno music generation.
//
// Suno has no official public API — uses third-party relays or API proxies.
// Async pattern: POST /v1/music/generate → task ID → GET /v1/music/{id} → poll
//
// Models: suno-v5, chirp-v5, suno-v4, chirp-v4
package suno

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("suno", &SunoExecutor{})
}

// SunoExecutor handles Suno music generation via relay/proxy API.
type SunoExecutor struct {
	channel any
}

func (e *SunoExecutor) Init(channel any) {
	e.channel = channel
}

func (e *SunoExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Suno"
}

func (e *SunoExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.suno.ai"
}

func (e *SunoExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// sunoGenerateRequest maps to Suno relay /v1/music/generate.
type sunoGenerateRequest struct {
	Prompt       string `json:"prompt"`
	Model        string `json:"model,omitempty"`
	Title        string `json:"title,omitempty"`
	Tags         string `json:"tags,omitempty"`
	Instrumental bool   `json:"instrumental,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	CallbackURL  string `json:"callback_url,omitempty"`
}

type sunoGenerateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID string `json:"id"`
	} `json:"data"`
	SunoID string `json:"suno_id,omitempty"`
	TaskID string `json:"task_id,omitempty"`
}

type sunoTaskResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		ID        string  `json:"id"`
		Status    string  `json:"status"`
		Title     string  `json:"title,omitempty"`
		AudioURL  string  `json:"audio_url,omitempty"`
		Lyric     string  `json:"lyric,omitempty"`
		Duration  float64 `json:"duration,omitempty"`
		Fail      string  `json:"fail,omitempty"`
	} `json:"data"`
}

func (e *SunoExecutor) TextToSpeech(_ *audio.TTSRequest) (*audio.AudioStream, error) {
	return nil, audio.ErrNotSupported
}

func (e *SunoExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *SunoExecutor) MusicGenerate(req *audio.MusicRequest) (*audio.AudioTask, error) {
	sReq := sunoGenerateRequest{
		Prompt:       req.Prompt,
		Model:        req.Model,
		Title:        req.Title,
		Tags:         req.Tags,
		Instrumental: req.Instrumental,
		Duration:     req.Duration,
		CallbackURL:  req.CallbackURL,
	}
	if sReq.Model == "" {
		sReq.Model = "suno-v5"
	}

	payload, err := json.Marshal(sReq)
	if err != nil {
		return nil, fmt.Errorf("suno: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/music/generate", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("suno: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("suno: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var sResp sunoGenerateResponse
	if err := json.Unmarshal(raw, &sResp); err != nil {
		return nil, fmt.Errorf("suno: unmarshal: %w", err)
	}

	taskID := sResp.Data.ID
	if taskID == "" {
		taskID = sResp.SunoID
	}
	if taskID == "" {
		taskID = sResp.TaskID
	}
	if taskID == "" {
		return nil, fmt.Errorf("suno: no task ID in response")
	}

	return &audio.AudioTask{
		ID:        taskID,
		Status:    audio.AudioTaskPending,
		CreatedAt: time.Now().Unix(),
	}, nil
}

func (e *SunoExecutor) GetTask(taskID string) (*audio.AudioTask, error) {
	reqURL := strings.TrimSuffix(e.getBaseURL(), "/") + "/v1/music/" + taskID
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("suno: create fetch req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("suno: fetch task: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("suno: read task: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("suno: task HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var tResp sunoTaskResponse
	if err := json.Unmarshal(raw, &tResp); err != nil {
		return nil, fmt.Errorf("suno: unmarshal task: %w", err)
	}

	task := &audio.AudioTask{
		ID: taskID,
	}
	switch tResp.Data.Status {
	case "completed", "success":
		task.Status = audio.AudioTaskCompleted
		task.Title = tResp.Data.Title
		task.AudioURL = tResp.Data.AudioURL
		task.Lyric = tResp.Data.Lyric
		task.Duration = tResp.Data.Duration
	case "failed", "error":
		task.Status = audio.AudioTaskFailed
		task.Error = tResp.Data.Fail
		if task.Error == "" {
			task.Error = tResp.Message
		}
	case "processing", "running":
		task.Status = audio.AudioTaskProcessing
	default:
		task.Status = audio.AudioTaskPending
	}

	return task, nil
}

func (e *SunoExecutor) ListVoices() ([]audio.Voice, error) {
	return nil, audio.ErrNotSupported
}

func (e *SunoExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("suno: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

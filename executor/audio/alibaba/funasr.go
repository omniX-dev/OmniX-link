// Package alibaba implements AudioExecutor for Alibaba FunASR STT.
//
// Cloud API: POST /api/v1/services/audio/asr/transcription (async, poll for result)
// Self-hosted: OpenAI-compatible POST /v1/audio/transcriptions (sync)
//
// FunASR is STT-only — no TTS support. Supports 30+ languages, speaker diarization.
package alibaba

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("funasr", &FunASRExecutor{})
}

// FunASRExecutor handles Alibaba FunASR speech recognition.
type FunASRExecutor struct {
	channel any
}

func (e *FunASRExecutor) Init(channel any) {
	e.channel = channel
}

func (e *FunASRExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "FunASR"
}

func (e *FunASRExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://dashscope.aliyuncs.com"
}

func (e *FunASRExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *FunASRExecutor) isSelfHosted() bool {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		url := ch.GetBaseURL()
		return url != "" && !strings.Contains(url, "dashscope")
	}
	return false
}

// funASRRequest maps to DashScope async ASR API.
type funASRRequest struct {
	Model string `json:"model"`
	Input struct {
		AudioURL string `json:"audio_url,omitempty"`
	} `json:"input"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type funASRResponse struct {
	Output struct {
		TaskID     string `json:"task_id,omitempty"`
		TaskStatus string `json:"task_status,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Result     string `json:"result,omitempty"`
	} `json:"output"`
}

func (e *FunASRExecutor) TextToSpeech(_ *audio.TTSRequest) (*audio.AudioStream, error) {
	return nil, audio.ErrNotSupported
}

func (e *FunASRExecutor) SpeechToText(req *audio.STTRequest) (*audio.STTResult, error) {
	// Self-hosted mode: use OpenAI-compatible /v1/audio/transcriptions
	if e.isSelfHosted() {
		return e.selfHostedSTT(req)
	}

	// Cloud API: async transcription via DashScope
	return e.cloudSTT(req)
}

func (e *FunASRExecutor) selfHostedSTT(req *audio.STTRequest) (*audio.STTResult, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	fw, err := w.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("funasr: create file: %w", err)
	}
	if _, err := fw.Write(req.File); err != nil {
		return nil, fmt.Errorf("funasr: write file: %w", err)
	}

	model := req.Model
	if model == "" {
		model = "fun-asr"
	}
	w.WriteField("model", model)
	if req.Language != "" {
		w.WriteField("language", req.Language)
	}
	w.Close()

	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	httpReq, err := http.NewRequest("POST", baseURL+"/v1/audio/transcriptions", &buf)
	if err != nil {
		return nil, fmt.Errorf("funasr: create req: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("funasr: do req: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("funasr: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("funasr: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var sttResp struct {
		Text     string `json:"text"`
		Language string `json:"language,omitempty"`
	}
	if err := json.Unmarshal(raw, &sttResp); err != nil {
		return nil, fmt.Errorf("funasr: unmarshal: %w", err)
	}

	return &audio.STTResult{
		Text:     sttResp.Text,
		Language: sttResp.Language,
	}, nil
}

func (e *FunASRExecutor) cloudSTT(req *audio.STTRequest) (*audio.STTResult, error) {
	fReq := funASRRequest{
		Model: req.Model,
	}
	if fReq.Model == "" {
		fReq.Model = "fun-asr"
	}

	// Need audio URL — if only raw bytes, upload first or use self-hosted
	if req.FileName != "" {
		// Expect audio to be accessible via URL in Extra
		if req.Extra != nil {
			if url, ok := req.Extra["audio_url"].(string); ok {
				fReq.Input.AudioURL = url
			}
		}
	}

	params := make(map[string]any)
	if req.Language != "" {
		params["language"] = req.Language
	}
	for k, v := range req.Extra {
		if k != "audio_url" {
			params[k] = v
		}
	}
	fReq.Parameters = params

	payload, err := json.Marshal(fReq)
	if err != nil {
		return nil, fmt.Errorf("funasr: marshal: %w", err)
	}

	resp, err := e.doRequest("/api/v1/services/audio/asr/transcription", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("funasr: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("funasr: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var fResp funASRResponse
	if err := json.Unmarshal(raw, &fResp); err != nil {
		return nil, fmt.Errorf("funasr: unmarshal: %w", err)
	}
	if fResp.Output.Code != "" {
		return nil, fmt.Errorf("funasr: %s: %s", fResp.Output.Code, fResp.Output.Message)
	}

	// Async — poll until done
	taskID := fResp.Output.TaskID
	if taskID == "" {
		return nil, fmt.Errorf("funasr: no task ID")
	}

	// Poll loop with timeout
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		task, err := e.queryTask(taskID)
		if err != nil {
			return nil, err
		}
		if task.Output.TaskStatus == "SUCCESS" || task.Output.TaskStatus == "SUCCEEDED" {
			result := &audio.STTResult{
				Text: task.Output.Result,
			}
			return result, nil
		}
		if task.Output.TaskStatus == "FAILED" {
			return nil, fmt.Errorf("funasr: task failed: %s", task.Output.Message)
		}
	}

	return nil, fmt.Errorf("funasr: transcription timeout")
}

type taskQueryResp struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Result     string `json:"result,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
	} `json:"output"`
}

func (e *FunASRExecutor) queryTask(taskID string) (*taskQueryResp, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/api/v1/tasks/"+taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("funasr: create task req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("funasr: query: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("funasr: read task: %w", err)
	}

	var tResp taskQueryResp
	if err := json.Unmarshal(raw, &tResp); err != nil {
		return nil, fmt.Errorf("funasr: unmarshal task: %w", err)
	}
	return &tResp, nil
}

func (e *FunASRExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *FunASRExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *FunASRExecutor) ListVoices() ([]audio.Voice, error) {
	return nil, audio.ErrNotSupported
}

func (e *FunASRExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("funasr: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

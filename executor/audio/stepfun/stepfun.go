// Package stepfun implements AudioExecutor for Stepfun (阶跃星辰) StepAudio TTS.
//
// Endpoint:
//   - POST /v1/audio/speech — TextToSpeech (OpenAI-compatible)
//
// Model: stepaudio-2.5-tts
package stepfun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("stepfun", &StepAudioExecutor{})
}

// StepAudioExecutor handles Stepfun StepAudio TTS.
type StepAudioExecutor struct {
	channel any
}

func (e *StepAudioExecutor) Init(channel any) {
	e.channel = channel
}

func (e *StepAudioExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "StepAudio"
}

func (e *StepAudioExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.stepfun.com/v1"
}

func (e *StepAudioExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// stepAudioRequest maps to the Stepfun TTS request body (OpenAI-compatible).
type stepAudioRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice,omitempty"`
	Instruction    string  `json:"instruction,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

func (e *StepAudioExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	stepReq := stepAudioRequest{
		Model: req.Model,
		Input: req.Input,
		Voice: req.Voice,
	}
	if stepReq.Model == "" {
		stepReq.Model = "stepaudio-2.5-tts"
	}
	if stepReq.Voice == "" {
		stepReq.Voice = "cixingnansheng"
	}

	// Map Instructions field
	if req.Instructions != "" {
		stepReq.Instruction = req.Instructions
	}

	// Map ResponseFormat
	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}
	stepReq.ResponseFormat = format

	// Map Speed
	if req.Speed > 0 {
		stepReq.Speed = req.Speed
	}

	payload, err := json.Marshal(stepReq)
	if err != nil {
		return nil, fmt.Errorf("stepaudio: marshal: %w", err)
	}

	resp, err := e.doRequest("/audio/speech", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("stepaudio: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stepaudio: read audio: %w", err)
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: "audio/" + format,
		Format:      format,
	}), nil
}

func (e *StepAudioExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *StepAudioExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *StepAudioExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *StepAudioExecutor) ListVoices() ([]audio.Voice, error) {
	return []audio.Voice{
		{ID: "cixingnansheng", Name: "磁性男声", Category: "premade"},
		{ID: "linjiameimei", Name: "邻家妹妹", Category: "premade"},
		{ID: "zhixingjiejie", Name: "知性姐姐", Category: "premade"},
		{ID: "gaolengyujie", Name: "高冷御姐", Category: "premade"},
		{ID: "tiantian", Name: "甜甜", Category: "premade"},
		{ID: "shusheng", Name: "书生", Category: "premade"},
		{ID: "xiannv", Name: "仙女", Category: "premade"},
		{ID: "boyi", Name: "播报", Category: "premade"},
	}, nil
}

func (e *StepAudioExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+"/"+strings.TrimPrefix(path, "/"), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("stepaudio: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

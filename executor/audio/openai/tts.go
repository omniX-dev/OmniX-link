// Package openai implements AudioExecutor for OpenAI TTS and STT.
//
// Endpoints:
//   - POST /v1/audio/speech — TextToSpeech
//   - POST /v1/audio/transcriptions — SpeechToText
//
// Models: gpt-4o-mini-tts, tts-1, tts-1-hd (TTS)
// Models: whisper-1, gpt-4o-transcribe, gpt-4o-mini-transcribe (STT)
package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("openai", &OpenAIAudioExecutor{})
}

// OpenAIAudioExecutor handles OpenAI TTS and STT.
type OpenAIAudioExecutor struct {
	channel any
}

func (e *OpenAIAudioExecutor) Init(channel any) {
	e.channel = channel
}

func (e *OpenAIAudioExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "OpenAI"
}

func (e *OpenAIAudioExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.openai.com"
}

func (e *OpenAIAudioExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// ttsRequest maps to OpenAI /v1/audio/speech.
type ttsRequest struct {
	Model          string  `json:"model,omitempty"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice,omitempty"`
	Instructions   string  `json:"instructions,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

func (e *OpenAIAudioExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	body := ttsRequest{
		Model:          req.Model,
		Input:          req.Input,
		Voice:          req.Voice,
		Instructions:   req.Instructions,
		ResponseFormat: req.ResponseFormat,
		Speed:          req.Speed,
	}
	if body.Model == "" {
		body.Model = "tts-1"
	}
	if body.Voice == "" {
		body.Voice = "coral"
	}
	if body.ResponseFormat == "" {
		body.ResponseFormat = "mp3"
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai-tts: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/audio/speech", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai-tts: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai-tts: read audio: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: contentType,
		Format:      body.ResponseFormat,
	}), nil
}

// sttResponse maps to OpenAI /v1/audio/transcriptions response.
type sttResponse struct {
	Text     string        `json:"text"`
	Language string        `json:"language,omitempty"`
	Duration float64       `json:"duration,omitempty"`
	Segments []interface{} `json:"segments,omitempty"`
	Words    []interface{} `json:"words,omitempty"`
}

func (e *OpenAIAudioExecutor) SpeechToText(req *audio.STTRequest) (*audio.STTResult, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// File field
	fw, err := w.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("openai-stt: create file field: %w", err)
	}
	if _, err := fw.Write(req.File); err != nil {
		return nil, fmt.Errorf("openai-stt: write file: %w", err)
	}

	// Model
	model := req.Model
	if model == "" {
		model = "whisper-1"
	}
	if err := w.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("openai-stt: write model: %w", err)
	}

	// Optional fields
	if req.Language != "" {
		w.WriteField("language", req.Language)
	}
	if req.Prompt != "" {
		w.WriteField("prompt", req.Prompt)
	}
	if req.ResponseFormat != "" {
		w.WriteField("response_format", req.ResponseFormat)
	}
	if req.Temperature > 0 {
		w.WriteField("temperature", fmt.Sprintf("%.2f", req.Temperature))
	}

	w.Close()

	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	httpReq, err := http.NewRequest("POST", baseURL+"/v1/audio/transcriptions", &buf)
	if err != nil {
		return nil, fmt.Errorf("openai-stt: create req: %w", err)
	}
	httpReq.Header.Set("Content-Type", w.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai-stt: do req: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai-stt: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai-stt: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var sttResp sttResponse
	if err := json.Unmarshal(raw, &sttResp); err != nil {
		return nil, fmt.Errorf("openai-stt: unmarshal: %w", err)
	}

	result := &audio.STTResult{
		Text:     sttResp.Text,
		Language: sttResp.Language,
		Duration: sttResp.Duration,
	}

	// Parse segments if present
	for _, s := range sttResp.Segments {
		if seg, ok := s.(map[string]any); ok {
			result.Segments = append(result.Segments, audio.STTSegment{
				Start: toFloat64(seg["start"]),
				End:   toFloat64(seg["end"]),
				Text:  toString(seg["text"]),
			})
		}
	}

	for _, w := range sttResp.Words {
		if word, ok := w.(map[string]any); ok {
			result.Words = append(result.Words, audio.STTWord{
				Word:  toString(word["word"]),
				Start: toFloat64(word["start"]),
				End:   toFloat64(word["end"]),
			})
		}
	}

	return result, nil
}

func (e *OpenAIAudioExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *OpenAIAudioExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *OpenAIAudioExecutor) ListVoices() ([]audio.Voice, error) {
	return nil, audio.ErrNotSupported
}

func (e *OpenAIAudioExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai-audio: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	client := &http.Client{}
	return client.Do(req)
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

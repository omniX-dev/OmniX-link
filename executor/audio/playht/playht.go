// Package playht implements AudioExecutor for PlayHT TTS.
//
// Endpoints:
//   - POST /v2/tts/stream — sync streaming TTS
//   - POST /v2/tts — async job TTS
//
// Auth: Authorization: Bearer + X-USER-ID
// Models: PlayHT2.0, Play3.0-mini, PlayDialog
package playht

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
	audio.RegisterAudio("playht", &PlayHTExecutor{})
}

// PlayHTExecutor handles PlayHT text-to-speech.
type PlayHTExecutor struct {
	channel any
}

func (e *PlayHTExecutor) Init(channel any) {
	e.channel = channel
}

func (e *PlayHTExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "PlayHT"
}

func (e *PlayHTExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.play.ht"
}

func (e *PlayHTExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func (e *PlayHTExecutor) getUserID() string {
	if ch, ok := e.channel.(interface{ GetUserID() string }); ok {
		return ch.GetUserID()
	}
	return ""
}

// playHTReq maps to PlayHT /v2/tts/stream request.
type playHTReq struct {
	Text     string `json:"text"`
	Voice    string `json:"voice,omitempty"`
	Model    string `json:"model,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Format   string `json:"output_format,omitempty"`
	SampleRate int   `json:"sample_rate,omitempty"`
}

func (e *PlayHTExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	voice := req.Voice
	if voice == "" {
		voice = "s3://voice-cloning-zero-shot/d9ff78ba-d016-47f6-b0ef-dd630f24314a/aditi/manifest.json"
	}

	pReq := playHTReq{
		Text:   req.Input,
		Voice:  voice,
		Model:  req.Model,
		Speed:  req.Speed,
		Format: req.ResponseFormat,
	}
	if pReq.Model == "" {
		pReq.Model = "PlayHT2.0-turbo"
	}
	if pReq.Format == "" {
		pReq.Format = "mp3"
	}

	payload, err := json.Marshal(pReq)
	if err != nil {
		return nil, fmt.Errorf("playht: marshal: %w", err)
	}

	resp, err := e.doRequest("/v2/tts/stream", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playht: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("playht: read audio: %w", err)
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: "audio/" + pReq.Format,
		Format:      pReq.Format,
	}), nil
}

func (e *PlayHTExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *PlayHTExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *PlayHTExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *PlayHTExecutor) ListVoices() ([]audio.Voice, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/v2/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("playht: create voice req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	req.Header.Set("X-User-ID", e.getUserID())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("playht: list voices: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("playht: read voices: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playht: voices HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var voicesResp []struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		PreviewURL string            `json:"preview_url,omitempty"`
		Labels     map[string]string `json:"labels,omitempty"`
		Category   string            `json:"category,omitempty"`
	}
	if err := json.Unmarshal(raw, &voicesResp); err != nil {
		return nil, fmt.Errorf("playht: unmarshal voices: %w", err)
	}

	var voices []audio.Voice
	for _, v := range voicesResp {
		voices = append(voices, audio.Voice{
			ID:         v.ID,
			Name:       v.Name,
			PreviewURL: v.PreviewURL,
			Labels:     v.Labels,
			Category:   v.Category,
		})
	}
	return voices, nil
}

func (e *PlayHTExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("playht: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	req.Header.Set("X-User-ID", e.getUserID())
	return (&http.Client{}).Do(req)
}

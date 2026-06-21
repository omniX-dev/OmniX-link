// Package fishaudio implements AudioExecutor for Fish Audio TTS.
//
// Endpoints:
//   - POST /v1/tts — text-to-speech
//   - POST /v1/voices/clone — voice cloning
//
// Model: fish-speech (S2, Qwen3-4B backbone)
// Features: Zero-shot cloning, 80+ languages, open source.
package fishaudio

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
	audio.RegisterAudio("fishaudio", &FishAudioExecutor{})
}

// FishAudioExecutor handles Fish Audio text-to-speech and voice cloning.
type FishAudioExecutor struct {
	channel any
}

func (e *FishAudioExecutor) Init(channel any) {
	e.channel = channel
}

func (e *FishAudioExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "FishAudio"
}

func (e *FishAudioExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://api.fish.audio"
}

func (e *FishAudioExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// fishTTSReq maps to Fish Audio /v1/tts request.
type fishTTSReq struct {
	Text      string `json:"text"`
	VoiceID   string `json:"voice_id,omitempty"`
	Model     string `json:"model,omitempty"`
	Format    string `json:"format,omitempty"`
	Speed     float64 `json:"speed,omitempty"`
	Language  string `json:"language,omitempty"`
}

type fishTTSResp struct {
	Audio      string `json:"audio,omitempty"`
	AudioURL   string `json:"audio_url,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
}

func (e *FishAudioExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	fReq := fishTTSReq{
		Text:    req.Input,
		VoiceID: req.Voice,
		Model:   req.Model,
		Format:  req.ResponseFormat,
		Speed:   req.Speed,
	}
	if fReq.Format == "" {
		fReq.Format = "mp3"
	}
	if req.Extra != nil {
		if l, ok := req.Extra["language"].(string); ok {
			fReq.Language = l
		}
	}

	payload, err := json.Marshal(fReq)
	if err != nil {
		return nil, fmt.Errorf("fishaudio: marshal: %w", err)
	}

	resp, err := e.doRequest("/v1/tts", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fishaudio: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	contentType := resp.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "audio/") {
		// Direct audio stream response
		audioData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("fishaudio: read audio: %w", err)
		}
		return audio.NewStreamFromResult(&audio.AudioResult{
			Audio:       audioData,
			ContentType: contentType,
			Format:      fReq.Format,
		}), nil
	}

	// JSON response with base64 audio or URL
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fishaudio: read: %w", err)
	}

	var fResp fishTTSResp
	if err := json.Unmarshal(raw, &fResp); err != nil {
		return nil, fmt.Errorf("fishaudio: unmarshal: %w", err)
	}

	if fResp.AudioURL != "" {
		// Fetch audio from URL
		audioResp, err := http.Get(fResp.AudioURL)
		if err != nil {
			return nil, fmt.Errorf("fishaudio: fetch audio: %w", err)
		}
		defer audioResp.Body.Close()
		audioData, err := io.ReadAll(audioResp.Body)
		if err != nil {
			return nil, fmt.Errorf("fishaudio: read fetched: %w", err)
		}
		return audio.NewStreamFromResult(&audio.AudioResult{
			Audio:       audioData,
			ContentType: "audio/" + fReq.Format,
			Format:      fReq.Format,
			Duration:    fResp.Duration,
		}), nil
	}

	return nil, fmt.Errorf("fishaudio: no audio in response")
}

func (e *FishAudioExecutor) SpeechToText(_ *audio.STTRequest) (*audio.STTResult, error) {
	return nil, audio.ErrNotSupported
}

func (e *FishAudioExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *FishAudioExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *FishAudioExecutor) ListVoices() ([]audio.Voice, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/v1/voices", nil)
	if err != nil {
		return nil, fmt.Errorf("fishaudio: create voice req: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fishaudio: list voices: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fishaudio: read voices: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fishaudio: voices HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var voicesResp []struct {
		ID      string            `json:"id"`
		Name    string            `json:"name"`
		Labels  map[string]string `json:"labels,omitempty"`
	}
	if err := json.Unmarshal(raw, &voicesResp); err != nil {
		return nil, fmt.Errorf("fishaudio: unmarshal voices: %w", err)
	}

	var voices []audio.Voice
	for _, v := range voicesResp {
		voices = append(voices, audio.Voice{
			ID:     v.ID,
			Name:   v.Name,
			Labels: v.Labels,
		})
	}
	return voices, nil
}

func (e *FishAudioExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("fishaudio: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

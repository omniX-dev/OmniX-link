// Package azure implements AudioExecutor for Azure Speech Services.
//
// Endpoints:
//   - TTS: {region}.tts.speech.microsoft.com/cognitiveservices/v1
//   - STT: {region}.stt.speech.microsoft.com/speech/recognition/conversation/cognitiveservices/v1
//
// Features: 400+ multilingual voices, SSML support, custom neural voices.
package azure

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/audio"
)

func init() {
	audio.RegisterAudio("azure", &AzureExecutor{})
}

// AzureExecutor handles Microsoft Azure Speech Services TTS and STT.
type AzureExecutor struct {
	channel any
}

func (e *AzureExecutor) Init(channel any) {
	e.channel = channel
}

func (e *AzureExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Azure"
}

func (e *AzureExecutor) getRegion() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		url := ch.GetBaseURL()
		if url != "" {
			return url
		}
	}
	return "eastus"
}

func (e *AzureExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// voiceMap maps standard voice names to Azure voice names.
func voiceToAzure(voice string) string {
	if voice == "" {
		return "zh-CN-XiaoxiaoNeural"
	}
	// If already Azure format, use as-is
	if strings.Contains(voice, "Neural") || strings.Contains(voice, "-") {
		return voice
	}
	// Known short names
	switch voice {
	case "xiaoxiao":
		return "zh-CN-XiaoxiaoNeural"
	case "xiaoyi":
		return "zh-CN-XiaoyiNeural"
	case "yunxi":
		return "zh-CN-YunxiNeural"
	case "yunye":
		return "zh-CN-YunyeNeural"
	case "jenny":
		return "en-US-JennyNeural"
	case "guy":
		return "en-US-GuyNeural"
	case "aria":
		return "en-US-AriaNeural"
	case "davis":
		return "en-US-DavisNeural"
	default:
		return voice
	}
}

func (e *AzureExecutor) buildSSML(text, voice, lang string) string {
	if lang == "" {
		lang = "zh-CN"
	}
	if strings.HasPrefix(voice, "zh-") || strings.HasPrefix(voice, "en-") {
		lang = voice[:5]
	}
	return fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="%s">
	<voice name="%s">%s</voice>
</speak>`, lang, voice, text)
}

func (e *AzureExecutor) TextToSpeech(req *audio.TTSRequest) (*audio.AudioStream, error) {
	voice := voiceToAzure(req.Voice)
	lang := req.Extra["lang"]
	langStr, _ := lang.(string)
	ssml := e.buildSSML(req.Input, voice, langStr)

	format := req.ResponseFormat
	if format == "" {
		format = "mp3"
	}

	region := e.getRegion()
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", region)

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader([]byte(ssml)))
	if err != nil {
		return nil, fmt.Errorf("azure: create req: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/ssml+xml")
	httpReq.Header.Set("X-Microsoft-OutputFormat", audioFormatToAzure(format))
	httpReq.Header.Set("Ocp-Apim-Subscription-Key", e.getAPIKey())
	httpReq.Header.Set("User-Agent", "Omni-link")

	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure: do req: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure: read audio: %w", err)
	}

	return audio.NewStreamFromResult(&audio.AudioResult{
		Audio:       audioData,
		ContentType: "audio/" + format,
		Format:      format,
	}), nil
}

func audioFormatToAzure(f string) string {
	switch f {
	case "mp3":
		return "audio-24khz-48kbitrate-mono-mp3"
	case "wav":
		return "riff-24khz-16bit-mono-pcm"
	case "opus":
		return "audio-24khz-48kbitrate-mono-mp3"
	case "pcm":
		return "raw-24khz-16bit-mono-pcm"
	default:
		return "audio-24khz-48kbitrate-mono-mp3"
	}
}

func (e *AzureExecutor) SpeechToText(req *audio.STTRequest) (*audio.STTResult, error) {
	region := e.getRegion()
	lang := req.Language
	if lang == "" {
		lang = "zh-CN"
	}

	url := fmt.Sprintf("https://%s.stt.speech.microsoft.com/speech/recognition/conversation/cognitiveservices/v1?language=%s", region, lang)

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(req.File))
	if err != nil {
		return nil, fmt.Errorf("azure: create stt req: %w", err)
	}

	contentType := "audio/wav"
	if req.FileName != "" {
		if strings.HasSuffix(req.FileName, ".mp3") {
			contentType = "audio/mpeg"
		} else if strings.HasSuffix(req.FileName, ".opus") {
			contentType = "audio/opus"
		}
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Ocp-Apim-Subscription-Key", e.getAPIKey())

	resp, err := (&http.Client{}).Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure: stt req: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure: read stt: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure: STT HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var azResp struct {
		DisplayText string `json:"DisplayText"`
		Duration    float64 `json:"Duration,omitempty"`
	}
	if err := fmt.Errorf("%s", string(raw)); err != nil && azResp.DisplayText == "" {
		// Try raw text response
		return &audio.STTResult{Text: strings.TrimSpace(string(raw))}, nil
	}

	// JSON response
	if azResp.DisplayText != "" {
		return &audio.STTResult{Text: azResp.DisplayText, Duration: azResp.Duration}, nil
	}

	return &audio.STTResult{Text: strings.TrimSpace(string(raw))}, nil
}

func (e *AzureExecutor) MusicGenerate(_ *audio.MusicRequest) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *AzureExecutor) GetTask(_ string) (*audio.AudioTask, error) {
	return nil, audio.ErrNotSupported
}

func (e *AzureExecutor) ListVoices() ([]audio.Voice, error) {
	return nil, audio.ErrNotSupported
}

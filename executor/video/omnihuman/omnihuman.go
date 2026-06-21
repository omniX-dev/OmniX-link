// Package omnihuman implements VideoExecutor for ByteDance OmniHuman (via fal.ai).
//
// SPECIALIZED: Image + Audio → Talking avatar video (数字人像).
// Not general T2V — only ImageToVideo with audio in Extra.
//
// Endpoint: POST /fal-ai/bytedance/omnihuman/v1.5
// Parameters: image_url + audio_url + optional prompt
// Max 60s at 720p, 30s at 1080p.
package omnihuman

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
	video.RegisterVideo("omnihuman", &OmniHumanExecutor{})
}

// OmniHumanExecutor handles ByteDance OmniHuman avatar generation.
type OmniHumanExecutor struct {
	channel any
}

func (e *OmniHumanExecutor) Init(channel any) {
	e.channel = channel
}

func (e *OmniHumanExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "OmniHuman"
}

func (e *OmniHumanExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://fal.run"
}

func (e *OmniHumanExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

type omniFalResp struct {
	RequestID string `json:"request_id,omitempty"`
	Status    string `json:"status"`
	Output    *struct {
		VideoURL string `json:"video_url,omitempty"`
	} `json:"output,omitempty"`
	Error *struct {
		Message string `json:"message,omitempty"`
	} `json:"error,omitempty"`
}

func (e *OmniHumanExecutor) TextToVideo(_ *video.TextToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *OmniHumanExecutor) ImageToVideo(req *video.ImageToVideoRequest) (*video.VideoTask, error) {
	falReq := map[string]any{
		"image_url": req.Image,
	}
	if req.Prompt != "" {
		falReq["prompt"] = req.Prompt
	}

	// Audio URL required — passed via Extra
	if req.Extra != nil {
		if audioURL, ok := req.Extra["audio_url"].(string); ok {
			falReq["audio_url"] = audioURL
		}
		for k, v := range req.Extra {
			if k != "audio_url" {
				falReq[k] = v
			}
		}
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("omnihuman: marshal: %w", err)
	}

	resp, err := e.doRequest("/fal-ai/bytedance/omnihuman/v1.5", payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("omnihuman: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("omnihuman: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp omniFalResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("omnihuman: unmarshal: %w", err)
	}

	task := &video.VideoTask{
		ID:        falResp.RequestID,
		Status:    video.VideoTaskQueued,
		CreatedAt: time.Now().Unix(),
	}

	switch falResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if falResp.Output != nil {
			task.VideoURL = falResp.Output.VideoURL
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if falResp.Error != nil {
			task.Error = falResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *OmniHumanExecutor) VideoToVideo(_ *video.VideoToVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *OmniHumanExecutor) ExtendVideo(_ *video.ExtendVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *OmniHumanExecutor) EditVideo(_ *video.EditVideoRequest) (*video.VideoTask, error) {
	return nil, video.ErrNotSupported
}

func (e *OmniHumanExecutor) CreateCharacter(_ *video.CharacterRequest) (*video.Character, error) {
	return nil, video.ErrNotSupported
}

func (e *OmniHumanExecutor) GetTask(taskID string) (*video.VideoTask, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("GET", baseURL+"/fal-ai/bytedance/omnihuman/v1.5/requests/"+taskID+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("omnihuman: create fetch: %w", err)
	}
	req.Header.Set("Authorization", "Key "+e.getAPIKey())

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("omnihuman: fetch: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("omnihuman: read fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("omnihuman: fetch HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp omniFalResp
	if err := json.Unmarshal(raw, &falResp); err != nil {
		return nil, fmt.Errorf("omnihuman: unmarshal fetch: %w", err)
	}

	task := &video.VideoTask{ID: taskID}
	switch falResp.Status {
	case "completed", "succeeded":
		task.Status = video.VideoTaskCompleted
		if falResp.Output != nil {
			task.VideoURL = falResp.Output.VideoURL
		}
	case "failed":
		task.Status = video.VideoTaskFailed
		if falResp.Error != nil {
			task.Error = falResp.Error.Message
		}
	case "processing", "in_progress":
		task.Status = video.VideoTaskProcessing
	}

	return task, nil
}

func (e *OmniHumanExecutor) doRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("omnihuman: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

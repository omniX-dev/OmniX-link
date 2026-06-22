// Package fal implements ImageExecutor for ByteDance Seedream via fal.ai.
//
// Endpoints:
//   - POST /fal-ai/seedream/{version}/text-to-image — T2I
//   - POST /fal-ai/seedream/{version}/image-to-image — I2I
//
// Auth: Authorization: Key <fal_key>
package fal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/just4zeroq/Omni-link/executor/image"
)

func init() {
	image.RegisterImage("fal", &FalExecutor{})
}

// FalExecutor handles ByteDance Seedream via fal.ai API.
type FalExecutor struct {
	channel any
}

func (e *FalExecutor) Init(channel any) {
	e.channel = channel
}

func (e *FalExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Fal"
}

func (e *FalExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://fal.ai"
}

func (e *FalExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

// falModelPath maps model name → fal.ai T2I endpoint path.
func falModelPath(model string) string {
	switch model {
	case "seedream-5.0":
		return "/fal-ai/seedream/5.0/text-to-image"
	case "seedream-4.5":
		return "/fal-ai/seedream/4.5/text-to-image"
	case "seedream-4.0":
		return "/fal-ai/seedream/text-to-image"
	default:
		return "/fal-ai/seedream/text-to-image"
	}
}

func falModelPathI2I(model string) string {
	switch model {
	case "seedream-5.0":
		return "/fal-ai/seedream/5.0/image-to-image"
	case "seedream-4.5":
		return "/fal-ai/seedream/4.5/image-to-image"
	default:
		return "/fal-ai/seedream/image-to-image"
	}
}

func (e *FalExecutor) doFalRequest(path string, payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("fal: create req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Key "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

func (e *FalExecutor) parseFalResponse(resp *http.Response) (*image.ImageTask, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fal: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fal: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var falResp struct {
		Images []struct {
			URL     string `json:"url,omitempty"`
			B64JSON string `json:"content,omitempty"`
			Width   int    `json:"width,omitempty"`
			Height  int    `json:"height,omitempty"`
		} `json:"images,omitempty"`
		Detail       string `json:"detail,omitempty"`
		Seed         int64  `json:"seed,omitempty"`
		ErrorMessage string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(raw, &falResp); err != nil {
		// Try top-level URL format
		var simpleResp struct {
			Image struct {
				URL string `json:"url,omitempty"`
			} `json:"image,omitempty"`
			URL    string `json:"url,omitempty"`
			Detail string `json:"detail,omitempty"`
			Seed   int64  `json:"seed,omitempty"`
		}
		if err2 := json.Unmarshal(raw, &simpleResp); err2 != nil {
			return nil, fmt.Errorf("fal: unmarshal: %w", err)
		}
		task := &image.ImageTask{
			Status: image.TaskStatusCompleted,
		}
		imgURL := simpleResp.URL
		if imgURL == "" && simpleResp.Image.URL != "" {
			imgURL = simpleResp.Image.URL
		}
		if imgURL != "" {
			task.Images = append(task.Images, image.ImageResult{
				URL:  imgURL,
				Seed: simpleResp.Seed,
			})
		}
		if simpleResp.Detail != "" {
			task.Error = simpleResp.Detail
		}
		return task, nil
	}

	if falResp.ErrorMessage != "" {
		return nil, fmt.Errorf("fal: %s", falResp.ErrorMessage)
	}

	task := &image.ImageTask{
		Status: image.TaskStatusCompleted,
	}
	for _, img := range falResp.Images {
		url := img.URL
		if url == "" && img.B64JSON != "" {
			url = "data:image/png;base64," + img.B64JSON
		}
		task.Images = append(task.Images, image.ImageResult{
			URL:  url,
			Seed: falResp.Seed,
		})
	}

	return task, nil
}

func (e *FalExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "seedream-4.5"
	}

	falReq := map[string]any{
		"prompt": req.Prompt,
	}
	if req.Size != "" {
		falReq["image_size"] = req.Size
	}
	if req.N > 1 {
		falReq["num_images"] = req.N
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("fal: marshal: %w", err)
	}

	path := falModelPath(model)
	resp, err := e.doFalRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return e.parseFalResponse(resp)
}

func (e *FalExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	model := req.Model
	if model == "" {
		model = "seedream-4.5"
	}

	falReq := map[string]any{
		"prompt":    req.Prompt,
		"image_url": req.Image,
	}
	if req.Strength > 0 {
		falReq["strength"] = req.Strength
	}
	for k, v := range req.Extra {
		falReq[k] = v
	}

	payload, err := json.Marshal(falReq)
	if err != nil {
		return nil, fmt.Errorf("fal: marshal i2i: %w", err)
	}

	path := falModelPathI2I(model)
	resp, err := e.doFalRequest(path, payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return e.parseFalResponse(resp)
}

func (e *FalExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

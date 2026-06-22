// Package volcengine implements ImageExecutor for ByteDance Seedream via Volcengine Ark.
//
// Endpoint: POST /api/v3/images/generations
// Base URL: https://ark.cn-beijing.volces.com
// Models: doubao-seedream-5-0-260128, doubao-seedream-4-5-251128, doubao-seedream-4-0-250828
// Accepts short names: seedream-5.0, seedream-4.5, seedream-4.0
package volcengine

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
	image.RegisterImage("seedream", &SeedreamExecutor{})
}

// SeedreamExecutor handles ByteDance Seedream via Volcengine Ark API.
type SeedreamExecutor struct {
	channel any
}

func (e *SeedreamExecutor) Init(channel any) {
	e.channel = channel
}

func (e *SeedreamExecutor) GetName() string {
	if ch, ok := e.channel.(interface{ GetName() string }); ok {
		return ch.GetName()
	}
	return "Seedream"
}

func (e *SeedreamExecutor) getBaseURL() string {
	if ch, ok := e.channel.(interface{ GetBaseURL() string }); ok {
		if url := ch.GetBaseURL(); url != "" {
			return url
		}
	}
	return "https://ark.cn-beijing.volces.com"
}

func (e *SeedreamExecutor) getAPIKey() string {
	if ch, ok := e.channel.(interface{ GetAPIKey() string }); ok {
		return ch.GetAPIKey()
	}
	return ""
}

func volcModel(model string) string {
	switch model {
	case "seedream-5.0":
		return "doubao-seedream-5-0-260128"
	case "seedream-4.5":
		return "doubao-seedream-4-5-251128"
	case "seedream-4.0":
		return "doubao-seedream-4-0-250828"
	default:
		return model
	}
}

type volcImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url,omitempty"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	} `json:"data"`
}

func (e *SeedreamExecutor) doVolcRequest(payload []byte) (*http.Response, error) {
	baseURL := strings.TrimSuffix(e.getBaseURL(), "/")
	req, err := http.NewRequest("POST", baseURL+"/api/v3/images/generations", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("seedream: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.getAPIKey())
	return (&http.Client{}).Do(req)
}

func (e *SeedreamExecutor) parseVolcResponse(resp *http.Response) (*image.ImageTask, error) {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("seedream: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seedream: HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var volcResp volcImageResponse
	if err := json.Unmarshal(raw, &volcResp); err != nil {
		return nil, fmt.Errorf("seedream: unmarshal response: %w", err)
	}

	task := &image.ImageTask{
		Status:    image.TaskStatusCompleted,
		CreatedAt: volcResp.Created,
	}
	for i, d := range volcResp.Data {
		task.Images = append(task.Images, image.ImageResult{
			Index:         i,
			URL:           d.URL,
			B64JSON:       d.B64JSON,
			RevisedPrompt: d.RevisedPrompt,
		})
	}
	return task, nil
}

func (e *SeedreamExecutor) TextToImage(req *image.TextToImageRequest) (*image.ImageTask, error) {
	body := map[string]any{
		"prompt": req.Prompt,
		"n":      req.N,
	}
	if body["n"].(int) == 0 {
		body["n"] = 1
	}
	if m := volcModel(req.Model); m != "" {
		body["model"] = m
	} else {
		body["model"] = "doubao-seedream-4-5-251128"
	}
	if req.Size != "" {
		body["size"] = req.Size
	}
	if req.Quality != "" {
		body["quality"] = req.Quality
	}
	rf := req.ResponseFormat
	if rf == "" {
		rf = "url"
	}
	body["response_format"] = rf

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("seedream: marshal request: %w", err)
	}

	resp, err := e.doVolcRequest(payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return e.parseVolcResponse(resp)
}

func (e *SeedreamExecutor) ImageToImage(req *image.ImageToImageRequest) (*image.ImageTask, error) {
	body := map[string]any{
		"prompt": req.Prompt,
		"n":      req.N,
	}
	if body["n"].(int) == 0 {
		body["n"] = 1
	}
	if m := volcModel(req.Model); m != "" {
		body["model"] = m
	} else {
		body["model"] = "doubao-seedream-4-5-251128"
	}
	body["response_format"] = "url"
	if req.Size != "" {
		body["size"] = req.Size
	}
	if req.Image != "" {
		body["image_url"] = req.Image
	}
	if req.Strength > 0 {
		body["strength"] = req.Strength
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("seedream: marshal i2i: %w", err)
	}

	resp, err := e.doVolcRequest(payload)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return e.parseVolcResponse(resp)
}

func (e *SeedreamExecutor) GetTask(_ string) (*image.ImageTask, error) {
	return nil, image.ErrNotSupported
}

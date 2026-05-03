package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Analyzer describes a single video frame as text. Implementations may target
// any OpenAI-compatible chat-completions backend (OpenAI cloud, vLLM, LM
// Studio, llama.cpp, ollama via /v1).
type Analyzer interface {
	Describe(ctx context.Context, jpeg []byte, prompt string) (string, error)
}

// OpenAIClient talks to any OpenAI-compatible /chat/completions endpoint.
// baseURL should be the API root (e.g. "https://api.openai.com/v1" or
// "http://localhost:8888/v1"). apiKey may be empty for local servers.
type OpenAIClient struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAIClient {
	return &OpenAIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type chatMessage struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAIClient) Describe(ctx context.Context, jpeg []byte, prompt string) (string, error) {
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{{
			Role: "user",
			Content: []contentPart{
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
				{Type: "text", Text: prompt},
			},
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai %d: %s", resp.StatusCode, string(raw))
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response missing choices: %s", string(raw))
	}
	return parsed.Choices[0].Message.Content, nil
}

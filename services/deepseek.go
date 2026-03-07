package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	deepseekBaseURL   = "https://api.deepseek.com/v1"
	deepseekChatModel = "deepseek-chat"
)

// ─── Request / Response shapes matching Deepseek's OpenAI-compatible API ─────

type deepseekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepseekChatRequest struct {
	Model    string            `json:"model"`
	Messages []deepseekMessage `json:"messages"`
	Stream   bool              `json:"stream"`
	// Optional controls — sensible defaults for interview context
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// deepseekStreamChunk mirrors the SSE delta format Deepseek returns.
type deepseekStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// ─── Public types ─────────────────────────────────────────────────────────────

// AIMessage is the provider-agnostic message shape the service accepts.
type AIMessage struct {
	Role    string
	Content string
}

// StreamChunk is yielded to the caller for every streamed token.
type StreamChunk struct {
	Content          string
	Done             bool
	PromptTokens     int
	CompletionTokens int
	Err              error
}

// ─── Service ──────────────────────────────────────────────────────────────────

type DeepseekService struct {
	apiKey     string
	httpClient *http.Client
}

func NewDeepseekService(apiKey string) *DeepseekService {
	return &DeepseekService{
		apiKey:     apiKey,
		httpClient: &http.Client{},
	}
}

// StreamChat sends the conversation to Deepseek and streams chunks back via the
// returned channel. The caller must drain the channel fully.
//
// messages should contain the full conversation history including the system
// prompt as the first message (role = "system").
func (s *DeepseekService) StreamChat(
	ctx context.Context,
	messages []AIMessage,
) (<-chan StreamChunk, error) {

	payload := deepseekChatRequest{
		Model:       deepseekChatModel,
		Stream:      true,
		MaxTokens:   2048,
		Temperature: 0.7,
		Messages:    make([]deepseekMessage, 0, len(messages)),
	}
	for _, m := range messages {
		payload.Messages = append(payload.Messages, deepseekMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		deepseekBaseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("deepseek: build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek: http request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("deepseek: non-200 status %d: %s", resp.StatusCode, string(b))
	}

	ch := make(chan StreamChunk, 32)

	go func() {
		defer resp.Body.Close()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		var promptTokens, completionTokens int

		for scanner.Scan() {
			line := scanner.Text()

			// SSE lines look like:  data: {...}   or   data: [DONE]
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				ch <- StreamChunk{
					Done:             true,
					PromptTokens:     promptTokens,
					CompletionTokens: completionTokens,
				}
				return
			}

			var chunk deepseekStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				ch <- StreamChunk{Err: fmt.Errorf("deepseek: unmarshal chunk: %w", err)}
				return
			}

			// Capture usage when present (some providers send it on the last chunk)
			if chunk.Usage != nil {
				promptTokens = chunk.Usage.PromptTokens
				completionTokens = chunk.Usage.CompletionTokens
			}

			if len(chunk.Choices) > 0 {
				content := chunk.Choices[0].Delta.Content
				if content != "" {
					select {
					case ch <- StreamChunk{Content: content}:
					case <-ctx.Done():
						ch <- StreamChunk{Err: ctx.Err()}
						return
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Err: fmt.Errorf("deepseek: scanner: %w", err)}
		}
	}()

	return ch, nil
}

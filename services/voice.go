package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

const (
	elevenLabsSTTURL = "https://api.elevenlabs.io/v1/speech-to-text"
	elevenLabsTTSURL = "https://api.elevenlabs.io/v1/text-to-speech/%s/stream"
)

type VoiceService struct {
	apiKey     string
	voiceID    string
	httpClient *http.Client
}

func NewVoiceService(apiKey, voiceID string) *VoiceService {
	return &VoiceService{
		apiKey:     apiKey,
		voiceID:    voiceID,
		httpClient: &http.Client{},
	}
}

func (v *VoiceService) Transcribe(ctx context.Context, audioData []byte, filename string) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("elevenlabs stt: create form file: %w", err)
	}
	if _, err = fw.Write(audioData); err != nil {
		return "", fmt.Errorf("elevenlabs stt: write audio: %w", err)
	}
	// Use scribe_v2 for best accuracy; switch to scribe_v2_realtime for lower latency
	_ = mw.WriteField("model_id", "scribe_v2")
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, elevenLabsSTTURL, &buf)
	if err != nil {
		return "", fmt.Errorf("elevenlabs stt: build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("xi-api-key", v.apiKey)

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("elevenlabs stt: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("elevenlabs stt: status %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("elevenlabs stt: decode: %w", err)
	}
	return result.Text, nil
}

func (v *VoiceService) SynthesizeStream(ctx context.Context, text string) (io.ReadCloser, error) {
	payload, _ := json.Marshal(map[string]any{
		"text":     text,
		"model_id": "eleven_turbo_v2", // low-latency TTS model
		"voice_settings": map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
		},
	})

	url := fmt.Sprintf(elevenLabsTTSURL, v.voiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", v.apiKey)
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs tts: status %d: %s", resp.StatusCode, b)
	}

	return resp.Body, nil
}

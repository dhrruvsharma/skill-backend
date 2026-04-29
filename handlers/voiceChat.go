// handlers/voice.go

package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VoiceChat godoc
// POST /api/v1/sessions/:id/voice
//
// Accepts multipart audio, returns:
//   1. SSE "transcript" event with the user's transcribed text
//   2. SSE "delta" events for AI text tokens (same as SendMessage)
//   3. SSE "audio_url" event with a signed URL to the TTS audio (or stream audio inline)
//
// Two response modes via Accept header:
//   - "text/event-stream"  → SSE with transcript + text deltas + audio URL
//   - "audio/mpeg"         → raw MP3 stream (simpler, for native clients)

func VoiceChat(db *gorm.DB, deepseekSvc *services.DeepseekService, voiceSvc *services.VoiceService) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := middleware.GetUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		sessionID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session id"})
			return
		}

		// ── Read audio upload ────────────────────────────────────────────────
		file, header, err := c.Request.FormFile("audio")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "audio file required"})
			return
		}
		defer file.Close()

		audioBytes := make([]byte, header.Size)
		if _, err := io.ReadFull(file, audioBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read audio"})
			return
		}

		ctx := c.Request.Context()

		// ── Step 1: Transcribe ───────────────────────────────────────────────
		transcript, err := voiceSvc.Transcribe(ctx, audioBytes, header.Filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "transcription failed: " + err.Error()})
			return
		}
		transcript = strings.TrimSpace(transcript)
		if transcript == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no speech detected"})
			return
		}

		// ── Step 2: Send to Deepseek (reuse existing service) ────────────────
		svc := services.NewInterviewService(db, deepseekSvc)
		_, streamCh, err := svc.SendMessageStream(ctx, sessionID, userID, transcript)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		// ── SSE response ─────────────────────────────────────────────────────
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		flusher, canFlush := c.Writer.(http.Flusher)
		writeSSE := func(eventType, payload string) {
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, payload)
			if canFlush {
				flusher.Flush()
			}
		}

		// Emit the transcript so the UI can display what the user said
		writeSSE("transcript", transcript)

		// ── Step 3: Collect AI text while streaming text deltas ──────────────
		var sb strings.Builder
		var promptTokens, completionTokens int

		for chunk := range streamCh {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if chunk.Err != nil {
				writeSSE("error", chunk.Err.Error())
				return
			}
			if chunk.Done {
				promptTokens = chunk.PromptTokens
				completionTokens = chunk.CompletionTokens
				break
			}
			sb.WriteString(chunk.Content)
			writeSSE("delta", chunk.Content) // text token for UI
		}

		fullText := sb.String()

		// Check for AI-initiated interview end
		cleanedText, ended := checkAndHandleEndInterview(ctx, svc, sessionID, userID, fullText, writeSSE)
		_ = ended

		// Persist assistant message
		assistantMsg, err := svc.FinalizeAssistantMessage(ctx, sessionID, cleanedText, promptTokens, completionTokens)
		if err != nil {
			writeSSE("error", "failed to save assistant message")
			return
		}
		_ = assistantMsg

		// ── Step 4: TTS ──────────────────────────────────────────────────────
		// Option A: stream audio bytes inline as base64 chunks
		// Option B (shown): signal the client to fetch /voice/audio endpoint
		//
		// For simplicity, synthesize and stream audio bytes as a final SSE event
		// encoded in base64. For production, upload to S3 and send a URL instead.
		ttsText := stripMarkdown(cleanedText)
		audioStream, err := voiceSvc.SynthesizeStream(ctx, ttsText)
		if err != nil {
			writeSSE("error", "tts failed: "+err.Error())
			return
		}
		defer audioStream.Close()

		// Stream audio in base64 chunks so the browser can play progressively
		import64 := streamAudioAsBase64(c, audioStream, writeSSE, canFlush, flusher)
		_ = import64

		writeSSE("done", `{"message":"stream complete"}`)
	}
}

// streamAudioAsBase64 reads audio from r and emits base64-encoded SSE "audio" events.
func streamAudioAsBase64(
	c *gin.Context,
	r io.Reader,
	writeSSE func(string, string),
	canFlush bool,
	flusher http.Flusher,
) error {

	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			writeSSE("audio", encoded)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

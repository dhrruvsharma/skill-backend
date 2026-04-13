package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/dhrruvsharma/skill-charge-backend/middleware"
	"github.com/dhrruvsharma/skill-charge-backend/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// VideoChat handles POST /api/v1/sessions/:id/video.
//
// Accepts a multipart video upload (field: "video"), then:
//  1. Saves the video to disk and records the path in the session.
//  2. Extracts audio via ffmpeg and transcribes with ElevenLabs STT.
//  3. Runs pigo face detection to flag multiple persons.
//  4. Feeds the transcript into the AI just like a text/voice message.
//  5. Streams back SSE events:
//     - "transcript"     – the transcribed user speech
//     - "proctoring_flag"– present only when >1 face detected (JSON payload)
//     - "delta"          – AI response tokens
//     - "audio"          – base64 TTS chunks
//     - "done"           – stream complete
func VideoChat(db *gorm.DB, deepseekSvc *services.DeepseekService, voiceSvc *services.VoiceService, videoSvc *services.VideoService) gin.HandlerFunc {
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

		// ── Read video upload ────────────────────────────────────────────────
		file, header, err := c.Request.FormFile("video")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "video file required"})
			return
		}
		defer file.Close()

		videoBytes, err := io.ReadAll(file)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read video"})
			return
		}

		ctx := c.Request.Context()

		ext := filepath.Ext(header.Filename)
		if ext == "" {
			ext = ".mp4"
		}

		// ── Step 1: Persist video ────────────────────────────────────────────
		videoPath, err := videoSvc.SaveVideo(sessionID.String(), ext, videoBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save video"})
			return
		}

		// ── Step 2: Transcribe audio from video ──────────────────────────────
		transcript, err := videoSvc.ExtractAudioAndTranscribe(ctx, videoPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "transcription failed: " + err.Error()})
			return
		}
		transcript = strings.TrimSpace(transcript)
		if transcript == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no speech detected in video"})
			return
		}

		// ── Step 3: Face detection ───────────────────────────────────────────
		multipleFaces, maxFaces, _ := videoSvc.DetectMultipleFaces(ctx, videoPath)

		// ── Step 4: Update session recording metadata ────────────────────────
		svc := services.NewInterviewService(db, deepseekSvc)
		_ = svc.UpdateSessionRecording(ctx, sessionID, userID, videoPath, multipleFaces, maxFaces)

		// ── Step 5: Send transcript to AI ────────────────────────────────────
		_, streamCh, err := svc.SendMessageStream(ctx, sessionID, userID, transcript)
		if err != nil {
			handleInterviewServiceError(c, err)
			return
		}

		// ── SSE setup ────────────────────────────────────────────────────────
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

		// Emit what the user said
		writeSSE("transcript", transcript)

		// Emit proctoring alert when multiple faces detected
		if multipleFaces {
			flagData, _ := json.Marshal(map[string]any{
				"type":      "multiple_faces",
				"max_faces": maxFaces,
				"timestamp": time.Now().UTC(),
			})
			writeSSE("proctoring_flag", string(flagData))
		}

		// ── Step 6: Stream AI response ───────────────────────────────────────
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
			writeSSE("delta", chunk.Content)
		}

		fullText := sb.String()

		if _, err := svc.FinalizeAssistantMessage(ctx, sessionID, fullText, promptTokens, completionTokens); err != nil {
			writeSSE("error", "failed to save assistant message")
			return
		}

		// ── Step 7: TTS ──────────────────────────────────────────────────────
		audioStream, err := voiceSvc.SynthesizeStream(ctx, stripMarkdown(fullText))
		if err != nil {
			writeSSE("error", "tts failed: "+err.Error())
			return
		}
		defer audioStream.Close()

		buf := make([]byte, 4096)
		for {
			n, readErr := audioStream.Read(buf)
			if n > 0 {
				writeSSE("audio", base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				break
			}
		}

		writeSSE("done", `{"message":"stream complete"}`)
	}
}

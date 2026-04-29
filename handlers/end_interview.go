package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/dhrruvsharma/skill-charge-backend/services"
)

const endInterviewMarker = "[END_INTERVIEW]"

// checkAndHandleEndInterview inspects the AI response text for [END_INTERVIEW].
// If found it:
//  1. Strips the marker from the content.
//  2. Ends the session via InterviewService.
//  3. Generates a report via DeepSeek.
//  4. Calls writeSSE to emit the "session_end" event with the report JSON.
//
// Returns the (possibly cleaned) content and whether the interview was ended.
func checkAndHandleEndInterview(
	ctx context.Context,
	svc *services.InterviewService,
	sessionID uuid.UUID,
	userID uuid.UUID,
	fullText string,
	writeSSE func(eventType, payload string),
) (cleanedText string, ended bool) {

	if !strings.Contains(fullText, endInterviewMarker) {
		return fullText, false
	}

	cleanedText = strings.ReplaceAll(fullText, endInterviewMarker, "")
	cleanedText = strings.TrimSpace(cleanedText)

	// End the session
	_, _ = svc.EndSession(ctx, sessionID, userID)

	// Generate report (best-effort — don't fail the response if it errors)
	report, err := svc.GenerateReport(ctx, sessionID, userID)
	if err != nil {
		writeSSE("session_end", `{"report":null,"error":"`+err.Error()+`"}`)
		return cleanedText, true
	}

	// Compact the report JSON to a single line so it doesn't break SSE framing.
	var compacted bytes.Buffer
	if json.Compact(&compacted, []byte(report)) == nil {
		report = compacted.String()
	}

	writeSSE("session_end", report)
	return cleanedText, true
}

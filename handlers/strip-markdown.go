package handlers

import (
	"regexp"
	"strings"
)

var (
	reBold           = regexp.MustCompile(`\*{1,3}(.+?)\*{1,3}`)
	reUnderline      = regexp.MustCompile(`_{1,2}(.+?)_{1,2}`)
	reCode           = regexp.MustCompile("`{1,3}[^`]*`{1,3}")
	reHeading        = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBullet         = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	reNumbered       = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	reBlockquote     = regexp.MustCompile(`(?m)^>\s+`)
	reHorizontalRule = regexp.MustCompile(`(?m)^[-*_]{3,}\s*$`)
	reLink           = regexp.MustCompile(`\[(.+?)\]\(.+?\)`)
	reImage          = regexp.MustCompile(`!\[.*?\]\(.*?\)`)
	reMultiSpace     = regexp.MustCompile(`[ \t]{2,}`)
	reMultiNewline   = regexp.MustCompile(`\n{3,}`)
)

// stripMarkdown removes markdown syntax so the text is safe for TTS.
func stripMarkdown(s string) string {
	s = reImage.ReplaceAllString(s, "")
	s = reLink.ReplaceAllString(s, "$1")      // keep link label
	s = reBold.ReplaceAllString(s, "$1")      // **text** → text
	s = reUnderline.ReplaceAllString(s, "$1") // __text__ → text
	s = reCode.ReplaceAllString(s, "")        // strip code blocks/spans
	s = reHeading.ReplaceAllString(s, "")     // ## Heading → Heading
	s = reBullet.ReplaceAllString(s, "")      // - item → item
	s = reNumbered.ReplaceAllString(s, "")    // 1. item → item
	s = reBlockquote.ReplaceAllString(s, "")
	s = reHorizontalRule.ReplaceAllString(s, "")
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = reMultiNewline.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

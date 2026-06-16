// Package markdown renders community content for the web and email
// (docs/architecture/email.md). HTML output is sanitized: scripts and
// iframes are stripped (MAIL-02).
package markdown

import (
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/microcosm-cc/bluemonday"
)

var sanitizer = bluemonday.UGCPolicy()

// HTML renders markdown to sanitized HTML.
func HTML(md string) string {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	doc := p.Parse([]byte(md))
	renderer := html.NewRenderer(html.RendererOptions{Flags: html.CommonFlags})
	unsafe := markdown.Render(doc, renderer)
	return string(sanitizer.SanitizeBytes(unsafe))
}

var (
	mdHeading = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdEmph    = regexp.MustCompile(`[*_]{1,3}`)
	mdLink    = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mdImage   = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
	mdCode    = regexp.MustCompile("`+")
)

// Text renders markdown to readable plain text for the email text part.
func Text(md string) string {
	s := mdImage.ReplaceAllString(md, "$1")
	s = mdLink.ReplaceAllString(s, "$1 ($2)")
	s = mdHeading.ReplaceAllString(s, "")
	s = mdEmph.ReplaceAllString(s, "")
	s = mdCode.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slug builds a URL slug from a title.
func Slug(title string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(title), "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "post"
	}
	return s
}

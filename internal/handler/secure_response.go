package handler

import (
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
)

const maxStylesheetBytes = 128 << 10

var permittedAttachmentMediaTypes = map[string]string{
	"application/json":         "application/json",
	"application/pdf":          "application/pdf",
	"application/vnd.ms-excel": "application/vnd.ms-excel",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/zip": "application/zip",
	"image/gif":       "image/gif",
	"image/jpeg":      "image/jpeg",
	"image/png":       "image/png",
	"image/webp":      "image/webp",
	"text/calendar":   "text/calendar; charset=utf-8",
	"text/csv":        "text/csv; charset=utf-8",
}

func writeAttachmentStream(w http.ResponseWriter, fileName, contentType string, size int64, data io.ReadCloser) {
	defer data.Close()
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = ""
	}
	safeContentType, allowed := permittedAttachmentMediaTypes[strings.ToLower(mediaType)]
	if !allowed {
		safeContentType = "application/octet-stream"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": sanitizeAttachmentFilename(fileName)})
	if disposition == "" {
		disposition = `attachment; filename="download"`
	}
	w.Header().Set("Content-Type", safeContentType)
	w.Header().Set("Content-Disposition", disposition)
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- the stream uses the same encoded attachment name, strict
	// media allowlist, nosniff, no-store, and document sandbox as writeAttachment.
	if _, err := io.Copy(w, data); err != nil {
		log.Warn().Err(err).Msg("handler: failed to stream attachment")
	}
}

// writeAttachment applies a single, auditable download policy to generated
// files. Unknown and active content types are forced to opaque bytes, while the
// filename is encoded with mime.FormatMediaType instead of concatenated into a
// response header.
func writeAttachment(w http.ResponseWriter, fileName, contentType string, data []byte) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = ""
	}
	safeContentType, allowed := permittedAttachmentMediaTypes[strings.ToLower(mediaType)]
	if !allowed {
		safeContentType = "application/octet-stream"
	}

	safeName := sanitizeAttachmentFilename(fileName)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": safeName})
	if disposition == "" {
		disposition = `attachment; filename="download"`
	}
	w.Header().Set("Content-Type", safeContentType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- the response is an encoded attachment with a strict media
	// type allowlist, nosniff, no-store, and sandbox headers; HTML/SVG are forced
	// to application/octet-stream and cannot be interpreted as an HTML document.
	if _, err := w.Write(data); err != nil {
		log.Warn().Err(err).Msg("handler: failed to stream attachment")
	}
}

func sanitizeAttachmentFilename(fileName string) string {
	fileName = strings.ReplaceAll(fileName, `\`, "/")
	fileName = path.Base(strings.TrimSpace(fileName))
	fileName = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, fileName)
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || fileName == "." {
		return "download"
	}
	if len(fileName) > 200 {
		fileName = fileName[:200]
		for !utf8.ValidString(fileName) {
			fileName = fileName[:len(fileName)-1]
		}
	}
	return fileName
}

// writeStylesheet prevents a stylesheet response from being content-sniffed as
// HTML. Raw HTML sentinels and oversized/non-UTF-8 output are rejected before
// response headers are committed.
func writeStylesheet(w http.ResponseWriter, css string) {
	if len(css) > maxStylesheetBytes || !utf8.ValidString(css) || containsHTMLSentinel(css) {
		writeError(w, http.StatusInternalServerError, "Branding CSS is invalid", "")
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	// #nosec G705 -- output is size/UTF-8/HTML-sentinel validated above and is
	// served only as CSS with nosniff and a restrictive document sandbox.
	if _, err := w.Write([]byte(css)); err != nil {
		log.Warn().Err(err).Msg("handler: failed to stream branding stylesheet")
	}
}

func containsHTMLSentinel(value string) bool {
	lower := strings.ToLower(value)
	return strings.ContainsRune(value, '\x00') || strings.Contains(lower, "<script") ||
		strings.Contains(lower, "</style") || strings.Contains(lower, "<!--") || strings.Contains(lower, "-->")
}

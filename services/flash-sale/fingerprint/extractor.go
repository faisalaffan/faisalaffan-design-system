package fingerprint

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type DeviceFingerprint struct {
	UserAgent         string
	AcceptLanguage    string
	Platform          string
	PlatformModel     string
	RemoteAddr        string
	ClientFingerprint string // X-Device-Fingerprint header
}

// Extract builds a fingerprint from the request with fallback priority:
// 1. X-Device-Fingerprint header (client-provided, highest trust)
// 2. Server-side composite: User-Agent + Accept-Language + Platform + Model + IP
func Extract(r *http.Request) string {
	fp := r.Header.Get("X-Device-Fingerprint")
	if fp != "" {
		return "client:" + fp
	}
	// Server-side fallback
	parts := []string{
		r.UserAgent(),
		r.Header.Get("Accept-Language"),
		r.Header.Get("Sec-CH-UA-Platform"),
		r.Header.Get("Sec-CH-UA-Model"),
		r.RemoteAddr,
	}
	joined := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(joined))
	return "server:" + fmt.Sprintf("%x", hash[:16])
}

// ExtractFromGin extracts fingerprint from a Gin context.
func ExtractFromGin(c *gin.Context) string {
	return Extract(c.Request)
}

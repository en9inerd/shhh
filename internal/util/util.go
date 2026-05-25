// Package util provides small string-sanitization helpers shared across packages.
package util

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"unicode/utf8"
)

// GenerateID returns a random 32-character lowercase hex string (UUID v4 format).
func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b), nil
}

// StripControl removes ASCII control characters (0x00-0x1F and 0x7F) from s.
func StripControl(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r != 0x7f {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SanitizeFilename strips control characters and caps the result at 255 UTF-8
// bytes without splitting a multi-byte rune.
func SanitizeFilename(name string) string {
	return TruncateUTF8(StripControl(name), 255)
}

// TruncateUTF8 shortens s to at most maxBytes UTF-8 bytes without splitting a
// multi-byte rune. If s is already within the limit, it is returned unchanged.
func TruncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	// Scan back from maxBytes to find the start of a rune.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

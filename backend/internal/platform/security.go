package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var secretName = regexp.MustCompile(`(?i)(password|token|secret|private[_-]?key|authorization)`)

func NewEnrollmentToken(size int) (plain, hash string, err error) {
	if size < 16 {
		return "", "", errors.New("enrollment token must be at least 16 bytes")
	}
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", "", err
	}
	plain = hex.EncodeToString(value)
	digest := sha256.Sum256([]byte(plain))
	return plain, hex.EncodeToString(digest[:]), nil
}

func HashToken(plain string) string {
	digest := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(digest[:])
}
func TokenUsable(expires time.Time, used bool, now time.Time) bool {
	return !used && now.Before(expires)
}

func Redact(fields map[string]string) map[string]string {
	result := make(map[string]string, len(fields))
	for key, value := range fields {
		if secretName.MatchString(key) {
			result[key] = "[REDACTED]"
		} else {
			result[key] = value
		}
	}
	return result
}

func AllowedPath(root, path string) bool {
	base, err1 := filepath.Abs(root)
	clean, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	return clean == base || strings.HasPrefix(clean, base+string(filepath.Separator))
}
func AllowedSubject(subject, runnerID string) bool {
	return subject == "glyphflow.orders."+runnerID || subject == "glyphflow.events."+runnerID
}

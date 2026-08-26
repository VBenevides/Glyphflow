package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
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
	base, err1 := canonicalPath(root)
	clean, err2 := canonicalPath(path)
	if err1 != nil || err2 != nil {
		return false
	}
	return clean == base || strings.HasPrefix(clean, base+string(filepath.Separator))
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var suffix []string
	for current := filepath.Clean(abs); ; current = filepath.Dir(current) {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) || filepath.Dir(current) == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
	}
}
func AllowedSubject(subject, runnerID string) bool {
	return subject == "glyphflow.orders."+runnerID || subject == "glyphflow.events."+runnerID || subject == "glyphflow.heartbeats."+runnerID || subject == "glyphflow.control."+runnerID
}

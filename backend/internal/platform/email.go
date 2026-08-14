package platform

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
)

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("email is required")
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return "", errors.New("email is invalid")
	}
	return value, nil
}

func ParseEmailList(value string) ([]string, error) {
	seen := map[string]bool{}
	var emails []string
	for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' || unicode.IsSpace(r) }) {
		email, err := NormalizeEmail(item)
		if err != nil {
			return nil, err
		}
		if !seen[email] {
			seen[email] = true
			emails = append(emails, email)
		}
	}
	return emails, nil
}

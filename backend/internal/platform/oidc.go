package platform

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"
)

type OIDCClaims struct {
	Issuer   string
	Subject  string
	Audience []string
	Nonce    string
	Expires  time.Time
}

func ValidateOIDCClaims(claims OIDCClaims, issuer, audience, nonce string, now time.Time) error {
	if issuer == "" || audience == "" || nonce == "" || claims.Issuer != issuer || claims.Subject == "" {
		return errors.New("OIDC claims do not match configured identity")
	}
	if !claims.Expires.After(now) {
		return errors.New("OIDC token has expired")
	}
	if !contains(claims.Audience, audience) || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return errors.New("OIDC audience or nonce is invalid")
	}
	return nil
}

func ValidateOIDCCallback(callback, configured string) error {
	got, err := url.Parse(callback)
	if err != nil || got.Scheme != "https" || got.Host == "" {
		return errors.New("OIDC callback must use HTTPS")
	}
	for _, allowed := range strings.Split(configured, ",") {
		if strings.TrimSpace(allowed) == got.String() {
			return nil
		}
	}
	return errors.New("OIDC callback is not configured")
}

func PKCEChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

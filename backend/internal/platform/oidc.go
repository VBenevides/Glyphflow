package platform

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
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
	Username string
	Email    string
	Groups   []string
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
	return validateOIDCCallback(callback, configured, false)
}

func ValidateOIDCCallbackWithHTTP(callback, configured string) error {
	return validateOIDCCallback(callback, configured, true)
}

func validateOIDCCallback(callback, configured string, allowHTTP bool) error {
	got, err := url.Parse(callback)
	if err != nil || got.Host == "" || (got.Scheme != "https" && (!allowHTTP || got.Scheme != "http")) {
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

func VerifyOIDCIDToken(token, jwks, issuer, audience, nonce string, now time.Time) (OIDCClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return OIDCClaims{}, errors.New("OIDC ID token is malformed")
	}
	header, err := decodeOIDCHeader(parts[0])
	if err != nil {
		return OIDCClaims{}, err
	}
	claims, err := decodeOIDCClaims(parts[1], now)
	if err != nil {
		return OIDCClaims{}, err
	}
	if err := ValidateOIDCClaims(claims, issuer, audience, nonce, now); err != nil {
		return OIDCClaims{}, err
	}
	if err := verifyOIDCSignature(parts[0]+"."+parts[1], parts[2], header.Alg, header.Kid, jwks); err != nil {
		return OIDCClaims{}, err
	}
	return claims, nil
}

func decodeOIDCHeader(encoded string) (struct{ Alg, Kid string }, error) {
	headerBytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return struct{ Alg, Kid string }{}, errors.New("OIDC ID token header is malformed")
	}
	var header struct{ Alg, Kid string }
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Kid == "" {
		return struct{ Alg, Kid string }{}, errors.New("OIDC ID token header is invalid")
	}
	return header, nil
}

func decodeOIDCClaims(encoded string, now time.Time) (OIDCClaims, error) {
	payloadBytes, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return OIDCClaims{}, errors.New("OIDC ID token claims are malformed")
	}
	var payload struct {
		Issuer   string          `json:"iss"`
		Subject  string          `json:"sub"`
		Audience json.RawMessage `json:"aud"`
		Nonce    string          `json:"nonce"`
		Expires  json.Number     `json:"exp"`
		Username string          `json:"preferred_username"`
		Email    string          `json:"email"`
		Groups   []string        `json:"groups"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(payloadBytes)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return OIDCClaims{}, errors.New("OIDC ID token claims are invalid")
	}
	var audiences []string
	if len(payload.Audience) > 0 && payload.Audience[0] == '"' {
		var value string
		if err := json.Unmarshal(payload.Audience, &value); err != nil {
			return OIDCClaims{}, errors.New("OIDC ID token audience is invalid")
		}
		audiences = []string{value}
	} else if err := json.Unmarshal(payload.Audience, &audiences); err != nil {
		return OIDCClaims{}, errors.New("OIDC ID token audience is invalid")
	}
	expires, err := payload.Expires.Float64()
	if err != nil || expires <= float64(now.Unix()) {
		return OIDCClaims{}, errors.New("OIDC token has expired")
	}
	return OIDCClaims{Issuer: payload.Issuer, Subject: payload.Subject, Audience: audiences, Nonce: payload.Nonce, Expires: time.Unix(int64(expires), 0), Username: payload.Username, Email: payload.Email, Groups: append([]string(nil), payload.Groups...)}, nil
}

func verifyOIDCSignature(signingInput, encodedSignature, algorithm, keyID, jwks string) error {
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil {
		return errors.New("OIDC ID token signature is malformed")
	}
	var document struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal([]byte(jwks), &document); err != nil {
		return errors.New("OIDC JWKS is invalid")
	}
	for _, rawKey := range document.Keys {
		verified, err := verifyOIDCKey(signingInput, signature, algorithm, keyID, rawKey)
		if err != nil {
			return err
		}
		if verified {
			return nil
		}
	}
	return errors.New("OIDC ID token signature is invalid")
}

func verifyOIDCKey(signingInput string, signature []byte, algorithm, keyID string, rawKey json.RawMessage) (bool, error) {
	var key struct {
		KTY string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
		Crv string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if json.Unmarshal(rawKey, &key) != nil || key.Kid != keyID || (key.Alg != "" && key.Alg != algorithm) || (key.Use != "" && key.Use != "sig") {
		return false, nil
	}
	input := []byte(signingInput)
	switch algorithm {
	case "RS256":
		if key.KTY != "RSA" {
			return false, nil
		}
		n, nErr := decodeBigInt(key.N)
		e, eErr := decodeBigInt(key.E)
		if nErr != nil || eErr != nil || !e.IsInt64() {
			return false, nil
		}
		digest := sha256.Sum256(input)
		// RS256 is defined by JOSE to use PKCS#1 v1.5; do not change this without
		// deliberately breaking existing SSO provider compatibility.
		return rsa.VerifyPKCS1v15(&rsa.PublicKey{N: n, E: int(e.Int64())}, crypto.SHA256, digest[:], signature) == nil, nil // NOSONAR
	case "ES256", "ES384":
		curve, size := elliptic.P256(), 32
		if algorithm == "ES384" {
			curve, size = elliptic.P384(), 48
		}
		if key.KTY != "EC" || key.Crv != curve.Params().Name || len(signature) != size*2 {
			return false, nil
		}
		x, xErr := decodeBigInt(key.X)
		y, yErr := decodeBigInt(key.Y)
		if xErr != nil || yErr != nil || !curve.IsOnCurve(x, y) {
			return false, nil
		}
		var digest []byte
		if algorithm == "ES256" {
			hashValue := sha256.Sum256(input)
			digest = hashValue[:]
		} else {
			hashValue := sha512.Sum384(input)
			digest = hashValue[:]
		}
		return ecdsa.Verify(&ecdsa.PublicKey{Curve: curve, X: x, Y: y}, digest, new(big.Int).SetBytes(signature[:size]), new(big.Int).SetBytes(signature[size:])), nil
	default:
		return false, errors.New("OIDC signing algorithm is not supported")
	}
}

func decodeBigInt(value string) (*big.Int, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return nil, errors.New("invalid JWK integer")
	}
	return new(big.Int).SetBytes(decoded), nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

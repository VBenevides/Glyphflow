package platform

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

type PasswordHasher struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
	Pepper  []byte
}

func DefaultPasswordHasher(pepper []byte) PasswordHasher {
	return PasswordHasher{Time: 2, Memory: 64 * 1024, Threads: 2, KeyLen: 32, SaltLen: 16, Pepper: append([]byte(nil), pepper...)}
}

func (h PasswordHasher) validate() error {
	if h.Time == 0 || h.Memory < 8*uint32(h.Threads) || h.Threads == 0 || h.KeyLen == 0 || h.SaltLen < 16 {
		return errors.New("invalid Argon2id parameters")
	}
	return nil
}

func (h PasswordHasher) Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password is required")
	}
	if err := h.validate(); err != nil {
		return "", err
	}
	salt := make([]byte, h.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), append(salt, h.Pepper...), h.Time, h.Memory, h.Threads, h.KeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", h.Memory, h.Time, h.Threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (h PasswordHasher) Verify(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("malformed Argon2id hash")
	}
	params := map[string]uint64{}
	for _, item := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) != 2 {
			return false, errors.New("malformed Argon2id parameters")
		}
		value, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return false, errors.New("malformed Argon2id parameter")
		}
		params[pair[0]] = value
	}
	timeCost, okTime := params["t"]
	memory, okMemory := params["m"]
	threads, okThreads := params["p"]
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	want, errHash := base64.RawStdEncoding.DecodeString(parts[5])
	if !okTime || !okMemory || !okThreads || err != nil || errHash != nil || len(salt) < 16 || len(want) == 0 || timeCost == 0 || memory == 0 || threads == 0 {
		return false, errors.New("malformed Argon2id hash")
	}
	got := argon2.IDKey([]byte(password), append(salt, h.Pepper...), uint32(timeCost), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (h PasswordHasher) NeedsRehash(encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return true
	}
	params := map[string]string{}
	for _, item := range strings.Split(parts[3], ",") {
		pair := strings.SplitN(item, "=", 2)
		if len(pair) == 2 {
			params[pair[0]] = pair[1]
		}
	}
	return params["m"] != strconv.FormatUint(uint64(h.Memory), 10) || params["t"] != strconv.FormatUint(uint64(h.Time), 10) || params["p"] != strconv.FormatUint(uint64(h.Threads), 10)
}

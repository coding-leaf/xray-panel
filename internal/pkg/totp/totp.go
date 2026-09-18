package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	defaultPeriod = 30
	defaultDigits = 6
)

var b32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateOpts holds options for generating a TOTP key.
type GenerateOpts struct {
	Issuer      string
	AccountName string
}

// Key represents a generated TOTP key.
type Key struct {
	secret string
	url    string
}

// Secret returns the base32 secret.
func (k *Key) Secret() string {
	return k.secret
}

// URL returns the otpauth:// URL.
func (k *Key) URL() string {
	return k.url
}

// Generate generates a new random TOTP key.
func Generate(opts GenerateOpts) (*Key, error) {
	if opts.Issuer == "" {
		return nil, errors.New("missing issuer")
	}
	if opts.AccountName == "" {
		return nil, errors.New("missing account name")
	}

	secretBytes := make([]byte, 20)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random secret: %w", err)
	}

	secret := b32Encoding.EncodeToString(secretBytes)

	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", opts.Issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")

	label := opts.Issuer + ":" + opts.AccountName
	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + label,
		RawQuery: v.Encode(),
	}

	return &Key{
		secret: secret,
		url:    u.String(),
	}, nil
}

// decodeSecret decodes base32 secret with or without padding and case-insensitively.
func decodeSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(secret))
	// Try unpadded first
	data, err := b32Encoding.DecodeString(strings.TrimRight(s, "="))
	if err == nil {
		return data, nil
	}
	// Try standard padded
	return base32.StdEncoding.DecodeString(s)
}

// GenerateCode calculates the 6-digit TOTP code for a given timestamp.
func GenerateCode(secret string, t time.Time) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", fmt.Errorf("invalid secret: %w", err)
	}

	counter := uint64(t.Unix() / defaultPeriod)
	return hotp(key, counter, defaultDigits)
}

// Validate validates a passcode against secret for current time with a +/- 1 step window.
func Validate(passcode, secret string) bool {
	cleanCode := strings.TrimSpace(passcode)
	if len(cleanCode) != defaultDigits {
		return false
	}

	key, err := decodeSecret(secret)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	currentStep := uint64(now / defaultPeriod)

	// Check window: previous, current, next step
	for _, step := range []uint64{currentStep - 1, currentStep, currentStep + 1} {
		code, err := hotp(key, step, defaultDigits)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(cleanCode), []byte(code)) == 1 {
			return true
		}
	}

	return false
}

// hotp implements RFC 4226 HMAC-based One-Time Password algorithm.
func hotp(key []byte, counter uint64, digits int) (string, error) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	h := mac.Sum(nil)

	offset := h[len(h)-1] & 0x0f
	binaryCode := (binary.BigEndian.Uint32(h[offset:offset+4]) & 0x7fffffff) % 1000000

	return fmt.Sprintf("%0*d", digits, binaryCode), nil
}

package xray

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

type RealityKeyPair struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	ShortID    string `json:"shortId"`
}

// GenerateRealityKeyPair 生成符合 Xray 规范的 x25519 Reality 密钥对与 ShortId
func GenerateRealityKeyPair() (*RealityKeyPair, error) {
	var privKey [32]byte
	if _, err := rand.Read(privKey[:]); err != nil {
		return nil, fmt.Errorf("read random bytes failed: %w", err)
	}

	// 限制私钥位以符合 Curve25519 规范
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64

	var pubKey [32]byte
	curve25519.ScalarBaseMult(&pubKey, &privKey)

	privStr := base64.RawURLEncoding.EncodeToString(privKey[:])
	pubStr := base64.RawURLEncoding.EncodeToString(pubKey[:])

	var shortBytes [8]byte
	_, _ = rand.Read(shortBytes[:])
	shortID := hex.EncodeToString(shortBytes[:])

	return &RealityKeyPair{
		PrivateKey: privStr,
		PublicKey:  pubStr,
		ShortID:    shortID,
	}, nil
}

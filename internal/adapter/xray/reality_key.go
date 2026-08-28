package xray

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

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

// DerivePublicKeyFromPrivate 从 Reality base64 私钥自动推导 x25519 对应公钥
func DerivePublicKeyFromPrivate(privStr string) string {
	if privStr == "" {
		return ""
	}
	privBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(privStr, "="))
	if err != nil {
		privBytes, err = base64.StdEncoding.DecodeString(privStr)
	}
	if err != nil || len(privBytes) != 32 {
		return ""
	}
	var privKey, pubKey [32]byte
	copy(privKey[:], privBytes)
	privKey[0] &= 248
	privKey[31] &= 127
	privKey[31] |= 64
	curve25519.ScalarBaseMult(&pubKey, &privKey)
	return base64.RawURLEncoding.EncodeToString(pubKey[:])
}

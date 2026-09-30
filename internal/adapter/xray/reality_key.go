package xray

import (
	"panel/internal/pkg/crypto"
)

type RealityKeyPair = crypto.RealityKeyPair

// GenerateRealityKeyPair 生成符合 Xray 规范的 x25519 Reality 密钥对与 ShortId
func GenerateRealityKeyPair() (*RealityKeyPair, error) {
	return crypto.GenerateRealityKeyPair()
}

// DerivePublicKeyFromPrivate 从 Reality base64 私钥自动推导 x25519 对应公钥
func DerivePublicKeyFromPrivate(privStr string) string {
	return crypto.DerivePublicKeyFromPrivate(privStr)
}

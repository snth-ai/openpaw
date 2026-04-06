package uid

import (
	"crypto/rand"
	"encoding/hex"
)

// New генерирует короткий уникальный ID (16 hex символов).
func New() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

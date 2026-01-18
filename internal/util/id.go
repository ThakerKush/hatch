package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func RandomID(prefix string) string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	if prefix == "" {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(buf))
}

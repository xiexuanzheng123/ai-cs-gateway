package chat

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(bytes[:]))
}

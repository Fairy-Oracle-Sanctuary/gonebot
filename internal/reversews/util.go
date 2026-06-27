package reversews

import (
	"crypto/rand"
	"encoding/hex"
)

func cryptoRand(b []byte) (int, error) { return rand.Read(b) }
func stdHex(b []byte) string            { return hex.EncodeToString(b) }
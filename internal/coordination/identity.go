package coordination

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

func randomIdentifier(prefix string) (string, error) {
	suffix, err := randomBytes(9)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, suffix), nil
}

func randomToken() (string, error) {
	return randomBytes(24)
}

func randomBytes(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

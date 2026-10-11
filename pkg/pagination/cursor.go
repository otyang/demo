package pagination

import (
	"encoding/base64"
	"fmt"
)

func EncodeCursor(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func DecodeCursor(cursor string) (string, error) {
	if cursor == "" {
		return "", fmt.Errorf("cursor cannot be empty")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", fmt.Errorf("decode cursor: %w", err)
	}
	if len(decoded) == 0 {
		return "", fmt.Errorf("cursor value cannot be empty")
	}
	return string(decoded), nil
}

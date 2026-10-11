package jwtutils

import (
	"encoding/json"
	"fmt"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

func GenerateTokenWithStruct(jsonPrivateKey string, claims any) (string, error) {
	// 1. Parse the Private Key
	key, err := jwk.ParseKey([]byte(jsonPrivateKey))
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	// 2. Convert the claims struct into a jwt.Token object
	// We marshal to JSON first to respect `json:"..."` tags on your struct
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}

	// Parse the JSON directly into a jwt.Token instance
	tok, err := jwt.Parse(payload, jwt.WithVerify(false), jwt.WithValidate(false))
	if err != nil {
		return "", fmt.Errorf("failed to create token from claims: %w", err)
	}

	// 3. Extract/Detect Algorithm
	alg, ok := key.Algorithm()
	if !ok {
		alg = jwa.RS256() // Fallback default
	}

	// 4. Sign the token
	// This automatically includes the 'kid' from the key in the header
	signed, err := jwt.Sign(tok, jwt.WithKey(alg, key))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return string(signed), nil
}

// SignTokenWithJWK takes a pre-constructed jwt.Token and signs it using a JWK JSON string.
func SignTokenWithJWK(jsonPrivateKey string, tok jwt.Token) (string, error) {
	// 1. Parse the Private Key from JSON
	key, err := jwk.ParseKey([]byte(jsonPrivateKey))
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	// 2. Extract the algorithm from the key, fallback to RS256 if not specified
	alg, ok := key.Algorithm()
	if !ok {
		alg = jwa.RS256()
	}

	// 3. Sign the token
	// This automatically adds the 'kid' (Key ID) from the JWK to the JWT header.
	signed, err := jwt.Sign(tok, jwt.WithKey(alg, key))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return string(signed), nil
}

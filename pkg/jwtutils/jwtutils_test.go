package jwtutils

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom claims struct to test GenerateTokenWithStruct
type CustomClaims struct {
	UserID string   `json:"userId"`
	Roles  []string `json:"roles"`
	jwt.Token
}

func TestJWTFlow(t *testing.T) {
	ctx := context.Background()

	t.Run("Generate and Validate with Static Public Key", func(t *testing.T) {
		// 1. Generate
		claims := map[string]any{
			"sub": "user-123",
			"iss": "my-app",
			"aud": "my-api",
			"iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		}

		tokenStr, err := GenerateTokenWithStruct(ExampleMyPrivateKey, claims)
		require.NoError(t, err)
		assert.NotEmpty(t, tokenStr)

		// 2. Validate
		parsedToken, err := ValidateJwtWithPublicKey(tokenStr, ExampleMyPublicKeyJSON, "my-app", "my-api")
		require.NoError(t, err)
		sub, ok := parsedToken.Subject()
		assert.True(t, ok, "Subject claim should exist")
		assert.Equal(t, "user-123", sub)
	})

	t.Run("Sign existing jwt.Token object", func(t *testing.T) {
		tokenStr, err := SignTokenWithJWK(ExampleMyPrivateKey, ExampleTok)
		require.NoError(t, err)

		parsedToken, err := ValidateJwtWithPublicKey(tokenStr, ExampleMyPublicKeyJSON, "my-app", "my-api")
		require.NoError(t, err)
		sub, ok := parsedToken.Subject()
		assert.True(t, ok, "Subject claim should exist")
		assert.Equal(t, "user-123", sub)

		var val string
		err = parsedToken.Get("custom-claim", &val)
		assert.NoError(t, err)
		assert.Equal(t, "hello-world", val)
	})

	t.Run("JWKS Verifier with Mock Server", func(t *testing.T) {
		// Create a local JWKS server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwks := map[string]any{
				"keys": []any{
					// We need to parse the JSON string back to a map for the mock response
					jsonRaw(ExampleMyPublicKeyJSON),
				},
			}
			json.NewEncoder(w).Encode(jwks)
		}))
		defer server.Close()

		// Initialize Verifier pointing to mock server
		verifier, err := NewJWKSVerifier(ctx, server.URL, "my-app", "my-api")
		require.NoError(t, err)

		// Create a valid token
		tokenStr, _ := SignTokenWithJWK(ExampleMyPrivateKey, ExampleTok)

		// Validate using the background-refreshed cache
		parsedToken, err := verifier.Validate(tokenStr)
		require.NoError(t, err)
		sub, ok := parsedToken.Subject()
		assert.True(t, ok, "Subject claim should exist")
		assert.Equal(t, "user-123", sub)
	})

	t.Run("Validation Failures", func(t *testing.T) {
		tokenStr, _ := SignTokenWithJWK(ExampleMyPrivateKey, ExampleTok)

		// Test wrong issuer
		_, err := ValidateJwtWithPublicKey(tokenStr, ExampleMyPublicKeyJSON, "wrong-issuer", "my-api")
		assert.Error(t, err, "Should fail due to issuer mismatch")

		// Test expired token (mocking an expired token)
		expiredTok, _ := jwt.NewBuilder().
			Expiration(time.Now().Add(-1 * time.Hour)).
			Build()
		expiredStr, _ := SignTokenWithJWK(ExampleMyPrivateKey, expiredTok)

		_, err = ValidateJwtWithPublicKey(expiredStr, ExampleMyPublicKeyJSON, "", "")
		assert.Error(t, err, "Should fail due to expiration")
	})
}

// Helper to unmarshal the example JSON for the mock server response
func jsonRaw(js string) any {
	var out any
	json.Unmarshal([]byte(js), &out)
	return out
}

// newJWKSServer serves the example public key as a JWKS document.
func newJWKSServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{jsonRaw(ExampleMyPublicKeyJSON)}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestJWKSVerifierRejectsInvalidTokens(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	verifier, err := NewJWKSVerifier(ctx, newJWKSServer(t).URL, "my-app", "my-api")
	require.NoError(t, err)

	sign := func(t *testing.T, build func(*jwt.Builder) *jwt.Builder) string {
		t.Helper()
		token, err := build(jwt.NewBuilder().Issuer("my-app").Audience([]string{"my-api"}).Subject("user-1")).Build()
		require.NoError(t, err)
		signed, err := SignTokenWithJWK(ExampleMyPrivateKey, token)
		require.NoError(t, err)
		return signed
	}
	withExpiry := func(b *jwt.Builder) *jwt.Builder { return b.Expiration(time.Now().Add(time.Hour)) }

	_, err = verifier.Validate(sign(t, withExpiry))
	require.NoError(t, err, "a well-formed token must validate")

	t.Run("issuer with trailing slash", func(t *testing.T) {
		_, err := verifier.Validate(sign(t, func(b *jwt.Builder) *jwt.Builder {
			return withExpiry(b).Issuer("my-app/")
		}))
		require.NoError(t, err)
	})
	t.Run("no audience claim is accepted, as WorkOS access tokens may omit it", func(t *testing.T) {
		token, err := withExpiry(jwt.NewBuilder().Issuer("my-app").Subject("user-1")).Build()
		require.NoError(t, err)
		signed, err := SignTokenWithJWK(ExampleMyPrivateKey, token)
		require.NoError(t, err)
		_, err = verifier.Validate(signed)
		require.NoError(t, err)
	})
	t.Run("missing exp", func(t *testing.T) {
		_, err := verifier.Validate(sign(t, func(b *jwt.Builder) *jwt.Builder { return b }))
		require.Error(t, err)
	})
	t.Run("wrong audience", func(t *testing.T) {
		_, err := verifier.Validate(sign(t, func(b *jwt.Builder) *jwt.Builder {
			return withExpiry(b).Audience([]string{"other-api"})
		}))
		require.Error(t, err)
	})
	t.Run("wrong issuer", func(t *testing.T) {
		_, err := verifier.Validate(sign(t, func(b *jwt.Builder) *jwt.Builder {
			return withExpiry(b).Issuer("other-issuer")
		}))
		require.Error(t, err)
	})
	t.Run("unknown signing key", func(t *testing.T) {
		otherKey, err := jwk.Import(mustRSAKey(t))
		require.NoError(t, err)
		require.NoError(t, otherKey.Set(jwk.KeyIDKey, "unknown-kid"))
		token, err := withExpiry(jwt.NewBuilder().Issuer("my-app").Audience([]string{"my-api"})).Build()
		require.NoError(t, err)
		signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), otherKey))
		require.NoError(t, err)
		_, err = verifier.Validate(string(signed))
		require.Error(t, err)
	})
}

func TestNewJWKSVerifierFailsWhenKeysCannotBeFetched(t *testing.T) {
	notFound := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(notFound.Close)

	previous := initialFetchTimeout
	initialFetchTimeout = 300 * time.Millisecond
	t.Cleanup(func() { initialFetchTimeout = previous })

	// No deadline on ctx: before the fix this waited forever.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	started := time.Now()
	_, err := NewJWKSVerifier(ctx, notFound.URL, "", "")
	require.Error(t, err)
	require.Less(t, time.Since(started), 5*time.Second, "must give up instead of waiting forever")
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

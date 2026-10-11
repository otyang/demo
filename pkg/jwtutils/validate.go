package jwtutils

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// initialFetchTimeout bounds how long NewJWKSVerifier waits for the first
// successful JWKS fetch. It is a variable so tests can shorten it.
var initialFetchTimeout = 15 * time.Second

type JWKSVerifier struct {
	cache    *jwk.Cache
	jwksURL  string
	issuer   string
	audience string
}

// NewJWKSVerifier fetches the keys at jwksURL and keeps them refreshed in the
// background until ctx is cancelled. It fails if the first fetch does not
// succeed within 15 seconds (or before ctx's own deadline).
func NewJWKSVerifier(ctx context.Context, jwksURL, issuer, audience string) (*JWKSVerifier, error) {
	c, err := jwk.NewCache(ctx, httprc.NewClient())
	if err != nil {
		return nil, fmt.Errorf("failed to create jwk cache: %w", err)
	}

	// Register waits for the first successful fetch. Without a deadline it
	// would wait forever when the URL is unreachable or returns bad keys,
	// because failed fetches are only retried after the refresh interval.
	// The deadline only bounds this wait; the cache lives as long as ctx.
	registerCtx, cancel := context.WithTimeout(ctx, initialFetchTimeout)
	defer cancel()
	if err := c.Register(
		registerCtx,
		jwksURL,
		jwk.WithMaxInterval(24*time.Hour),
		jwk.WithMinInterval(15*time.Minute),
	); err != nil {
		return nil, fmt.Errorf("initial JWKS fetch from %q failed: %w", jwksURL, err)
	}

	return &JWKSVerifier{
		cache:    c,
		jwksURL:  jwksURL,
		issuer:   issuer,
		audience: audience,
	}, nil
}

// Validate checks a token string against the cached JWKS. Tokens must carry
// an exp claim. When configured, the issuer must match (ignoring a trailing
// slash) and, if the token has an aud claim, it must include the audience.
// This matches how the WorkOS SDK validates AuthKit access tokens, which may
// omit aud.
func (v *JWKSVerifier) Validate(tokenString string) (jwt.Token, error) {
	keySet, err := v.cache.CachedSet(v.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve keys from cache: %w", err)
	}

	// 1. Initialize base options
	options := []jwt.ParseOption{
		jwt.WithKeySet(keySet),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30 * time.Second),
		jwt.WithRequiredClaim(jwt.ExpirationKey),
	}

	if v.issuer != "" {
		options = append(options, jwt.WithValidator(issuerValidator(v.issuer)))
	}
	if v.audience != "" {
		options = append(options, jwt.WithValidator(audienceIfPresentValidator(v.audience)))
	}

	token, err := jwt.Parse([]byte(tokenString), options...)
	if err != nil {
		return nil, fmt.Errorf("jwt validation failed: %w", err)
	}

	return token, nil
}

// ValidateJwtWithPublicKey validates a token against a static Public Key JSON string.
// Tokens must carry an exp claim. Issuer and Audience are optional; if provided
// as empty strings, they are ignored.
func ValidateJwtWithPublicKey(tokenString string, publicKeyJSON string, issuer, audience string) (jwt.Token, error) {
	// 1. Parse the Public Key string
	key, err := jwk.ParseKey([]byte(publicKeyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	// 2. Extract the algorithm safely
	alg, ok := key.Algorithm()
	if !ok {
		alg = jwa.RS256() // Default fallback
	}

	// 3. Prepare Parse Options
	options := []jwt.ParseOption{
		jwt.WithKey(alg, key),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30 * time.Second),
		jwt.WithRequiredClaim(jwt.ExpirationKey),
	}

	// 4. Conditionally add Issuer and Audience checks
	if issuer != "" {
		options = append(options, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		options = append(options, jwt.WithAudience(audience))
	}

	// 5. Parse and validate with the gathered options
	tok, err := jwt.Parse([]byte(tokenString), options...)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	return tok, nil
}

// issuerValidator requires the iss claim to equal issuer, ignoring a trailing
// slash on either side ("https://api.workos.com" and "https://api.workos.com/"
// are the same issuer).
func issuerValidator(issuer string) jwt.Validator {
	want := strings.TrimRight(issuer, "/")
	return jwt.ValidatorFunc(func(_ context.Context, token jwt.Token) error {
		got, ok := token.Issuer()
		if !ok || strings.TrimRight(got, "/") != want {
			return fmt.Errorf("issuer %q is not the trusted issuer", got)
		}
		return nil
	})
}

// audienceIfPresentValidator accepts tokens without an aud claim, and
// requires tokens that have one to include audience.
func audienceIfPresentValidator(audience string) jwt.Validator {
	return jwt.ValidatorFunc(func(_ context.Context, token jwt.Token) error {
		audiences, ok := token.Audience()
		if !ok || len(audiences) == 0 {
			return nil
		}
		if slices.Contains(audiences, audience) {
			return nil
		}
		return fmt.Errorf("audience %v does not include %q", audiences, audience)
	})
}

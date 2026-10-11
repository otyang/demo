package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/otyang/demo/pkg/httpx"
	"github.com/otyang/demo/pkg/jwtutils"
	"github.com/otyang/demo/pkg/response"
)

type authClaimsContextKey struct{}

// JWTClaims contains the identity and authorization claims read from a JWT.
type JWTClaims struct {
	Issuer  string `json:"iss"`
	Subject string `json:"sub"`
	// SessionID is the WorkOS session the access token belongs to.
	SessionID      string   `json:"sid"`
	UserExternalID string   `json:"user_external_id"`
	OrganizationID string   `json:"org_id"`
	Email          string   `json:"email"`
	Roles          []string `json:"roles"`
	Permissions    []string `json:"permissions"`
	Scopes         []string `json:"scopes"`
}

// HasRole reports whether the roles claim includes role.
func (c *JWTClaims) HasRole(role string) bool {
	return c != nil && slices.Contains(c.Roles, role)
}

func (c *JWTClaims) HasPermission(permission string) bool {
	return c != nil && slices.Contains(c.Permissions, permission)
}

func (c *JWTClaims) HasScope(scope string) bool {
	return c != nil && slices.Contains(c.Scopes, scope)
}

func (c *JWTClaims) IsAdmin() bool {
	return c.HasRole("admin")
}

// JWTVerifier validates signed tokens using the configured key source.
type JWTVerifier interface {
	Validate(token string) (jwt.Token, error)
}

// JWTAuthentication validates Bearer tokens and makes their claims available
// through AuthClaimsFromContext.
func JWTAuthentication(verifier JWTVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			if verifier == nil {
				return response.NewError(
					http.StatusInternalServerError,
					"AUTH_CONFIGURATION_ERROR",
					"Authentication is unavailable",
					nil,
					errors.New("JWT verifier is nil"),
				)
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				return response.NewUnauthorizedError(
					"Authorization header must use Bearer token authentication",
					nil,
				)
			}

			parsed, err := verifier.Validate(token)
			if err != nil {
				return response.NewUnauthorizedError("Bearer token is invalid or expired", err)
			}
			claims, err := claimsFromToken(parsed)
			if errors.Is(err, errMalformedClaims) {
				// The token is validly signed but its claims have the wrong
				// shape, for example "permissions" as a string. That is the
				// token's fault, not the server's.
				return response.NewUnauthorizedError("Bearer token claims are malformed", err)
			}
			if err != nil {
				return response.NewInternalServerError(fmt.Errorf("extract JWT claims: %w", err))
			}

			ctx := context.WithValue(r.Context(), authClaimsContextKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return nil
		})
	}
}

// AuthClaimsFromContext retrieves claims added by JWTAuthentication.
func AuthClaimsFromContext(ctx context.Context) (*JWTClaims, error) {
	claims, ok := ctx.Value(authClaimsContextKey{}).(*JWTClaims)
	if !ok || claims == nil {
		return nil, response.NewUnauthorizedError("Authentication claims are missing from the request context", nil)
	}
	return claims, nil
}

// TestClaims attaches fixed claims to every request instead of validating a
// Bearer token. It is a drop-in replacement for JWTAuthentication in local
// development and tests only: it performs no authentication at all.
func TestClaims(claims JWTClaims) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Give each request its own copy so handlers cannot mutate the shared value.
			requestClaims := claims
			requestClaims.Roles = slices.Clone(claims.Roles)
			requestClaims.Permissions = slices.Clone(claims.Permissions)
			requestClaims.Scopes = slices.Clone(claims.Scopes)
			ctx := context.WithValue(r.Context(), authClaimsContextKey{}, &requestClaims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission rejects requests whose authenticated claims lack permission.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
			claims, err := AuthClaimsFromContext(r.Context())
			if err != nil {
				return err
			}
			if !claims.HasPermission(permission) {
				return response.NewForbiddenError("Insufficient permissions", nil)
			}
			next.ServeHTTP(w, r)
			return nil
		})
	}
}

// RequireOrganization rejects requests without a non-empty organization ID claim.
func RequireOrganization(next http.Handler) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		claims, err := AuthClaimsFromContext(r.Context())
		if err != nil {
			return err
		}
		if strings.TrimSpace(claims.OrganizationID) == "" {
			return response.NewForbiddenErrorWithCode(
				"ORG_SETUP_REQUIRED",
				"A valid organization ID is required for this request",
				nil,
			)
		}
		next.ServeHTTP(w, r)
		return nil
	})
}

func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	token = strings.TrimSpace(token)
	return token, found && strings.EqualFold(scheme, "Bearer") && token != ""
}

// errMalformedClaims marks claims that do not decode into JWTClaims.
var errMalformedClaims = errors.New("malformed JWT claims")

func claimsFromToken(token jwt.Token) (*JWTClaims, error) {
	if token == nil {
		return nil, errors.New("verifier returned a nil token")
	}
	data, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("marshal token: %w", err)
	}
	var claims JWTClaims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, fmt.Errorf("%w: %w", errMalformedClaims, err)
	}
	return &claims, nil
}

var _ JWTVerifier = (*jwtutils.JWKSVerifier)(nil)

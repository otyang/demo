package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lestrrat-go/jwx/v3/jwt"
	"github.com/stretchr/testify/require"
)

type stubJWTVerifier struct {
	token jwt.Token
	err   error
	got   string
}

func (v *stubJWTVerifier) Validate(token string) (jwt.Token, error) {
	v.got = token
	return v.token, v.err
}

func TestJWTAuthenticationAddsClaimsToContext(t *testing.T) {
	token, err := jwt.NewBuilder().
		Issuer("issuer").
		Subject("user-1").
		Claim("org_id", "org-1").
		Claim("roles", []string{"user", "admin"}).
		Claim("permissions", []string{"beneficiaries:read"}).
		Claim("scopes", []string{"profile"}).
		Build()
	require.NoError(t, err)
	verifier := &stubJWTVerifier{token: token}
	called := false
	handler := JWTAuthentication(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims, err := AuthClaimsFromContext(r.Context())
		require.NoError(t, err)
		require.Equal(t, "user-1", claims.Subject)
		require.Equal(t, "org-1", claims.OrganizationID)
		require.True(t, claims.HasRole("admin"))
		require.True(t, claims.IsAdmin())
		require.True(t, claims.HasPermission("beneficiaries:read"))
		require.True(t, claims.HasScope("profile"))
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer signed-token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.True(t, called)
	require.Equal(t, "signed-token", verifier.got)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestJWTAuthenticationRejectsMissingOrMalformedBearerToken(t *testing.T) {
	for _, authorization := range []string{"", "Basic token", "Bearer ", "Bearer"} {
		t.Run(authorization, func(t *testing.T) {
			verifier := &stubJWTVerifier{}
			handler := JWTAuthentication(verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("handler should not be reached")
			}))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", authorization)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			require.Empty(t, verifier.got)
		})
	}
}

func TestJWTAuthenticationRejectsInvalidToken(t *testing.T) {
	verifier := &stubJWTVerifier{err: errors.New("signature details")}
	handler := JWTAuthentication(verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not be reached")
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "signature details")
}

func TestRequirePermission(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	})
	handler := RequirePermission("beneficiaries:read")(next)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(contextWithClaims(t, &JWTClaims{
		Permissions: []string{"beneficiaries:read"},
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.True(t, nextCalled)
	require.Equal(t, http.StatusOK, recorder.Code)

	nextCalled = false
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(contextWithClaims(t, &JWTClaims{}))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.False(t, nextCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestRequireOrganization(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	handler := RequireOrganization(next)

	for _, test := range []struct {
		name           string
		organizationID string
		wantStatus     int
	}{
		{name: "organization present", organizationID: "org-1", wantStatus: http.StatusOK},
		{name: "organization whitespace", organizationID: "  ", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request = request.WithContext(contextWithClaims(t, &JWTClaims{OrganizationID: test.organizationID}))
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			require.Equal(t, test.wantStatus, recorder.Code)
		})
	}
}

func contextWithClaims(t *testing.T, claims *JWTClaims) context.Context {
	t.Helper()
	return context.WithValue(context.Background(), authClaimsContextKey{}, claims)
}

func TestTestClaimsAddsIndependentClaimsPerRequest(t *testing.T) {
	fixed := JWTClaims{OrganizationID: "org-test", Permissions: []string{"beneficiaries:read"}}
	handler := TestClaims(fixed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := AuthClaimsFromContext(r.Context())
		require.NoError(t, err)
		require.Equal(t, "org-test", claims.OrganizationID)
		require.True(t, claims.HasPermission("beneficiaries:read"))
		claims.Permissions[0] = "mutated"
		w.WriteHeader(http.StatusNoContent)
	}))

	for range 2 {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusNoContent, recorder.Code)
	}
	require.Equal(t, "beneficiaries:read", fixed.Permissions[0], "handlers must not mutate the shared claims")
}

func TestAuthRejectionsAreLogged(t *testing.T) {
	logs := captureLogs(t)

	handler := RequirePermission("beneficiaries:read")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called without claims")
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/beneficiaries", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	require.Equal(t, "WARN", entry["level"])
	require.Equal(t, "request failed", entry["msg"])
	require.Equal(t, float64(http.StatusUnauthorized), entry["status"])
	require.Equal(t, "/api/v1/beneficiaries", entry["path"])
	require.Equal(t, "Authentication claims are missing from the request context", entry["error_message"])
}

func TestHasRoleUsesRolesClaimOnly(t *testing.T) {
	token, err := jwt.NewBuilder().
		Subject("user-1").
		Claim("role", "admin"). // the single role claim is not used
		Claim("roles", []string{"member"}).
		Build()
	require.NoError(t, err)
	var claims *JWTClaims
	handler := JWTAuthentication(&stubJWTVerifier{token: token})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		claims, err = AuthClaimsFromContext(r.Context())
		require.NoError(t, err)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer token")
	handler.ServeHTTP(httptest.NewRecorder(), request)

	require.NotNil(t, claims)
	require.Equal(t, []string{"member"}, claims.Roles)
	require.True(t, claims.HasRole("member"))
	require.False(t, claims.HasRole("admin"))
	require.False(t, claims.IsAdmin())
}

func TestJWTAuthenticationRejectsMalformedClaimsAs401(t *testing.T) {
	token, err := jwt.NewBuilder().
		Subject("user-1").
		Claim("org_id", "org-1").
		Claim("permissions", "beneficiaries:read"). // a string, not a list
		Build()
	require.NoError(t, err)
	handler := JWTAuthentication(&stubJWTVerifier{token: token})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler called with malformed claims")
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Bearer token claims are malformed")
}

// captureLogs sends slog.Default() and the standard log package to a buffer
// for the rest of the test, restoring both afterwards. slog.SetDefault also
// redirects the log package, so restoring slog alone would leave log writing
// into this test's buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous, previousWriter, previousFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	})
	return &logs
}

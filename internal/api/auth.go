package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Authenticator resolves an authorization token to an exchange account.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (accountID int64, err error)
}

// StaticTokenAuthenticator is a single-user adapter for personal simulation.
// Multi-user deployments should replace it with a session or signed-token
// implementation.
type StaticTokenAuthenticator struct {
	tokenHash [sha256.Size]byte
	accountID int64
}

// NewStaticTokenAuthenticator creates a constant-time static bearer-token
// authenticator.
func NewStaticTokenAuthenticator(token string, accountID int64) (*StaticTokenAuthenticator, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("auth token is required")
	}
	if accountID <= 0 {
		return nil, errors.New("account ID must be positive")
	}
	return &StaticTokenAuthenticator{
		tokenHash: sha256.Sum256([]byte(token)),
		accountID: accountID,
	}, nil
}

func (a *StaticTokenAuthenticator) Authenticate(
	_ context.Context,
	token string,
) (int64, error) {
	if a == nil {
		return 0, errors.New("authenticator is unavailable")
	}
	provided := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(a.tokenHash[:], provided[:]) != 1 {
		return 0, errors.New("invalid token")
	}
	return a.accountID, nil
}

type authenticatedAccountKey struct{}

var accountIDContextKey authenticatedAccountKey

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg.Authenticator == nil {
			writeError(c, http.StatusServiceUnavailable, "authentication_unavailable",
				"API authentication is not configured")
			c.Abort()
			return
		}

		token, ok := bearerToken(c.Request)
		if !ok {
			c.Header("WWW-Authenticate", `Bearer realm="exchange"`)
			writeError(c, http.StatusUnauthorized, "unauthorized",
				"a bearer token is required")
			c.Abort()
			return
		}

		accountID, err := s.cfg.Authenticator.Authenticate(c.Request.Context(), token)
		if err != nil || accountID <= 0 {
			c.Header("WWW-Authenticate", `Bearer realm="exchange"`)
			writeError(c, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
			c.Abort()
			return
		}

		c.Set(accountIDContextKey, accountID)
		c.Next()
	}
}

func authenticatedAccountID(c *gin.Context) (int64, bool) {
	value, ok := c.Get(accountIDContextKey)
	if !ok {
		return 0, false
	}
	accountID, ok := value.(int64)
	return accountID, ok && accountID > 0
}

func bearerToken(request *http.Request) (string, bool) {
	if request == nil {
		return "", false
	}
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

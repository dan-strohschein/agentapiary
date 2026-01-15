// Package auth provides role-based access control (RBAC) for Apiary.
package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// AuthorizationConfig holds configuration for authorization middleware.
type AuthorizationConfig struct {
	// RequireAuth determines if authentication is required.
	// If true, requests without a principal will be rejected with 401 Unauthorized.
	// If false, requests without a principal are allowed (development/testing mode).
	// Default: true (require authentication)
	RequireAuth bool
}

// AuthorizationMiddleware creates middleware for authorization checks.
// By default, authentication is required. Requests without a principal will be rejected.
// If RequireAuth is false, unauthenticated requests are allowed (development/testing mode).
func AuthorizationMiddleware(rbac *RBAC, config ...AuthorizationConfig) echo.MiddlewareFunc {
	cfg := AuthorizationConfig{
		RequireAuth: true, // Default: require authentication
	}
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip authorization for non-namespaced endpoints (Cells)
			namespace := c.Param("namespace")
			if namespace == "" {
				return next(c)
			}

			// Get principal from context
			principal, ok := PrincipalFromContext(c.Request().Context())
			if !ok {
				// No principal - check if authentication is required
				if cfg.RequireAuth {
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error":   "unauthorized",
						"message": "Authentication required",
					})
				}
				// Development/testing mode: allow unauthenticated access
				return next(c)
			}

			// Principal exists - if RequireAuth is false (MVP mode), allow all requests
			if !cfg.RequireAuth {
				// MVP/development mode: allow all requests even if principal has no permissions
				return next(c)
			}

			// RequireAuth is true: check permissions
			// Determine permission based on HTTP method
			var permission Permission
			switch c.Request().Method {
			case http.MethodGet:
				permission = PermissionRead
			case http.MethodPost:
				permission = PermissionCreate
			case http.MethodPut, http.MethodPatch:
				permission = PermissionUpdate
			case http.MethodDelete:
				permission = PermissionDelete
			default:
				// Unknown method, allow (let handler deal with it)
				return next(c)
			}

			// Check permission
			if !rbac.CheckPermission(principal, namespace, permission) {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error":   "forbidden",
					"message": "Insufficient permissions",
				})
			}

			return next(c)
		}
	}
}

// ExtractPrincipalFromHeader extracts principal from HTTP headers (simple MVP approach).
//
// SECURITY WARNING: This is an unauthenticated, header-based approach that trusts
// user-provided headers without verification. This MUST NOT be used in production.
//
// In production, implement proper authentication:
//   - JWT validation with signature verification
//   - API keys with HMAC validation
//   - OAuth2/OIDC integration
//   - At minimum, add request signing/HMAC verification
//
// This MVP implementation should be removed before any production deployment.
func ExtractPrincipalFromHeader(c echo.Context) *Principal {
	// For MVP, use X-User-ID header (UNVERIFIED - DO NOT USE IN PRODUCTION)
	userID := c.Request().Header.Get("X-User-ID")
	if userID == "" {
		return nil
	}

	// Extract user name from header (optional)
	userName := c.Request().Header.Get("X-User-Name")
	if userName == "" {
		userName = userID
	}

	return &Principal{
		ID:   userID,
		Name: userName,
	}
}

// PrincipalMiddleware extracts principal from request and adds it to context.
func PrincipalMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			principal := ExtractPrincipalFromHeader(c)
			if principal != nil {
				ctx := WithPrincipal(c.Request().Context(), principal)
				c.SetRequest(c.Request().WithContext(ctx))
			}
			return next(c)
		}
	}
}

// RequireAuth is a helper to check if a principal exists in context.
func RequireAuth(c echo.Context) (*Principal, error) {
	principal, ok := PrincipalFromContext(c.Request().Context())
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Authentication required")
	}
	return principal, nil
}

// RequirePermission checks if a principal has a specific permission in a namespace.
func RequirePermission(rbac *RBAC, c echo.Context, namespace string, permission Permission) error {
	principal, err := RequireAuth(c)
	if err != nil {
		return err
	}

	if !rbac.CheckPermission(principal, namespace, permission) {
		return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
	}

	return nil
}

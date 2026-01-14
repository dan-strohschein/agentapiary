// Package auth provides role-based access control (RBAC) for Apiary.
package auth

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// AuthorizationMiddleware creates middleware for authorization checks.
// For MVP: If no principal is present, allow access (no authentication required).
// If principal is present, enforce RBAC permissions.
func AuthorizationMiddleware(rbac *RBAC) echo.MiddlewareFunc {
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
				// For MVP: If no principal, allow access (no authentication required)
				// In production, you'd want to require authentication
				return next(c)
			}

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
// In production, this would use proper authentication (JWT, API keys, etc.)
func ExtractPrincipalFromHeader(c echo.Context) *Principal {
	// For MVP, use X-User-ID header
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

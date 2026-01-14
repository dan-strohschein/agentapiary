package api

import (
	"net/http"

	"github.com/agentapiary/apiary/internal/auth"
	"github.com/labstack/echo/v4"
)

// assignRoleRequest represents a request to assign a role.
type assignRoleRequest struct {
	UserID string      `json:"userId"`
	Role   auth.Role   `json:"role"`
}

// assignRoleHandler handles POST /api/v1/cells/:namespace/roles.
func (s *Server) assignRoleHandler(c echo.Context) error {
	namespace := c.Param("namespace")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	if s.rbac == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "rbac_not_enabled",
			Message: "RBAC is not enabled",
		})
	}

	var req assignRoleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if err := s.rbac.AssignRole(namespace, req.UserID, req.Role); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	return c.NoContent(http.StatusCreated)
}

// listRolesHandler handles GET /api/v1/cells/:namespace/roles.
func (s *Server) listRolesHandler(c echo.Context) error {
	namespace := c.Param("namespace")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	if s.rbac == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "rbac_not_enabled",
			Message: "RBAC is not enabled",
		})
	}

	roles := s.rbac.ListCellRoles(namespace)
	return c.JSON(http.StatusOK, roles)
}

// removeRoleHandler handles DELETE /api/v1/cells/:namespace/roles/:userId.
func (s *Server) removeRoleHandler(c echo.Context) error {
	namespace := c.Param("namespace")
	userID := c.Param("userId")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	if s.rbac == nil {
		return c.JSON(http.StatusNotImplemented, errorResponse{
			Error:   "rbac_not_enabled",
			Message: "RBAC is not enabled",
		})
	}

	if err := s.rbac.RemoveRole(namespace, userID); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

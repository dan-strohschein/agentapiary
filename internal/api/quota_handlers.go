package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// getQuotaUsageHandler handles GET /api/v1/cells/:namespace/quota.
func (s *Server) getQuotaUsageHandler(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	usage, err := s.getQuotaUsage(ctx, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, usage)
}

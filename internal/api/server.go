// Package api provides the REST API server for Apiary.
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/agentapiary/apiary/internal/auth"
	"github.com/agentapiary/apiary/internal/comb"
	"github.com/agentapiary/apiary/internal/dlq"
	"github.com/agentapiary/apiary/internal/metrics"
	"github.com/agentapiary/apiary/internal/session"
	"github.com/agentapiary/apiary/pkg/apiary"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

// Server represents the API server.
type Server struct {
	echo          *echo.Echo
	store         apiary.ResourceStore
	sessionMgr    *session.Manager
	comb          comb.Comb
	metrics       *metrics.Collector
	rbac          *auth.RBAC
	dlqMgr        *dlq.Manager
	logger        *zap.Logger
	shutdown      chan struct{}
}

// Config holds server configuration.
type Config struct {
	Port        int
	Store       apiary.ResourceStore
	SessionMgr  *session.Manager
	Comb        comb.Comb
	Metrics     *metrics.Collector
	RBAC        *auth.RBAC
	DLQManager  *dlq.Manager
	Logger      *zap.Logger
}

// NewServer creates a new API server.
func NewServer(cfg Config) (*Server, error) {
	e := echo.New()
	e.HideBanner = true

	// Middleware
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	if cfg.Logger != nil {
		e.Use(loggingMiddleware(cfg.Logger))
	}
	
	// Context validation middleware
	e.Use(contextValidationMiddleware())

	// Principal extraction middleware (extracts user from headers)
	e.Use(auth.PrincipalMiddleware())

	// Authorization middleware (checks permissions)
	if cfg.RBAC != nil {
		e.Use(auth.AuthorizationMiddleware(cfg.RBAC))
	}

	// CORS
	e.Use(middleware.CORS())

	s := &Server{
		echo:       e,
		store:      cfg.Store,
		sessionMgr: cfg.SessionMgr,
		comb:       cfg.Comb,
		metrics:    cfg.Metrics,
		rbac:       cfg.RBAC,
		dlqMgr:     cfg.DLQManager,
		logger:     cfg.Logger,
		shutdown:   make(chan struct{}),
	}

	s.setupRoutes()

	return s, nil
}

// setupRoutes configures all API routes.
func (s *Server) setupRoutes() {
	v1 := s.echo.Group("/api/v1")

	// Cells (not namespaced)
	v1.GET("/cells", s.listCells)
	v1.GET("/cells/:name", s.getCell)
	v1.POST("/cells", s.createCell)
	v1.PUT("/cells/:name", s.updateCell)
	v1.DELETE("/cells/:name", s.deleteCell)

	// Namespaced resources
	ns := v1.Group("/cells/:namespace")

	// AgentSpecs
	ns.GET("/agentspecs", s.listAgentSpecs)
	ns.GET("/agentspecs/:name", s.getAgentSpec)
	ns.POST("/agentspecs", s.createAgentSpec)
	ns.PUT("/agentspecs/:name", s.updateAgentSpec)
	ns.DELETE("/agentspecs/:name", s.deleteAgentSpec)

	// Hives
	ns.GET("/hives", s.listHives)
	ns.GET("/hives/:name", s.getHive)
	ns.POST("/hives", s.createHive)
	ns.PUT("/hives/:name", s.updateHive)
	ns.DELETE("/hives/:name", s.deleteHive)

	// Secrets
	ns.GET("/secrets", s.listSecrets)
	ns.GET("/secrets/:name", s.getSecret)
	ns.POST("/secrets", s.createSecret)
	ns.PUT("/secrets/:name", s.updateSecret)
	ns.DELETE("/secrets/:name", s.deleteSecret)

	// Drones
	ns.GET("/drones", s.listDrones)
	ns.GET("/drones/:id", s.getDrone)
	ns.DELETE("/drones/:id", s.deleteDrone)

	// Sessions
	ns.GET("/sessions", s.listSessions)
	ns.GET("/sessions/:id", s.getSession)
	ns.POST("/hives/:hive/sessions", s.createSession)
	ns.POST("/sessions/:id/messages", s.sendMessage)
	ns.GET("/sessions/:id/memory", s.getSessionMemory)
	ns.DELETE("/sessions/:id", s.deleteSession)

	// Metrics
	v1.GET("/metrics", s.getMetrics)
	ns.GET("/metrics", s.getNamespaceMetrics)
	ns.GET("/metrics/:agentspec", s.getAgentSpecMetrics)

	// Quota
	ns.GET("/quota", s.getQuotaUsageHandler)

	// RBAC
	ns.GET("/roles", s.listRolesHandler)
	ns.POST("/roles", s.assignRoleHandler)
	ns.DELETE("/roles/:userId", s.removeRoleHandler)

	// DLQ (Dead Letter Queue)
	ns.GET("/hives/:hive/dlq", s.listDLQMessages)
	ns.GET("/hives/:hive/dlq/:messageId", s.getDLQMessage)
	ns.POST("/hives/:hive/dlq/:messageId/replay", s.replayDLQMessage)
	ns.DELETE("/hives/:hive/dlq/:messageId", s.deleteDLQMessage)
	ns.DELETE("/hives/:hive/dlq", s.clearDLQ)
}

// Start starts the API server.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// loggingMiddleware creates a logging middleware.
func loggingMiddleware(logger *zap.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:       true,
		LogError:     true,
		LogMethod:    true,
		LogLatency:   true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			logger.Info("request",
				zap.String("id", v.RequestID),
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
				zap.Error(v.Error),
			)
			return nil
		},
	})
}

// errorResponse represents an error response.
type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// contextValidationMiddleware checks if request context is cancelled before processing.
func contextValidationMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Context().Err() != nil {
				return c.JSON(http.StatusRequestTimeout, errorResponse{
					Error:   "request_timeout",
					Message: "Request context was cancelled",
				})
			}
			return next(c)
		}
	}
}

// handleError handles errors and returns appropriate HTTP responses.
func (s *Server) handleError(c echo.Context, err error) error {
	if err == apiary.ErrNotFound {
		return c.JSON(http.StatusNotFound, errorResponse{
			Error:   "not_found",
			Message: err.Error(),
		})
	}

	if err == apiary.ErrAlreadyExists {
		return c.JSON(http.StatusConflict, errorResponse{
			Error:   "already_exists",
			Message: err.Error(),
		})
	}

	if err == apiary.ErrInvalidInput {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_input",
			Message: err.Error(),
		})
	}

	s.logger.Error("internal server error", zap.Error(err))
	return c.JSON(http.StatusInternalServerError, errorResponse{
		Error:   "internal_error",
		Message: "An internal error occurred",
	})
}

// listCells handles GET /api/v1/cells.
func (s *Server) listCells(c echo.Context) error {
	ctx := c.Request().Context()
	resources, err := s.store.List(ctx, "Cell", "", nil)
	if err != nil {
		return s.handleError(c, err)
	}

	cells := make([]*apiary.Cell, 0, len(resources))
	for _, r := range resources {
		if cell, ok := r.(*apiary.Cell); ok {
			cells = append(cells, cell)
		}
	}

	return c.JSON(http.StatusOK, cells)
}

// getCell handles GET /api/v1/cells/:name.
func (s *Server) getCell(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.Param("name")

	resource, err := s.store.Get(ctx, "Cell", name, "")
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// createCell handles POST /api/v1/cells.
func (s *Server) createCell(c echo.Context) error {
	ctx := c.Request().Context()

	var cell apiary.Cell
	if err := c.Bind(&cell); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate namespace
	if err := validateNamespace(cell.GetName()); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	if err := s.store.Create(ctx, &cell); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusCreated, cell)
}

// updateCell handles PUT /api/v1/cells/:name.
func (s *Server) updateCell(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.Param("name")

	var cell apiary.Cell
	if err := c.Bind(&cell); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if cell.GetName() != name {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "name in URL does not match name in body",
		})
	}

	if err := s.store.Update(ctx, &cell); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, cell)
}

// deleteCell handles DELETE /api/v1/cells/:name.
func (s *Server) deleteCell(c echo.Context) error {
	ctx := c.Request().Context()
	name := c.Param("name")

	if err := s.store.Delete(ctx, "Cell", name, ""); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// listAgentSpecs handles GET /api/v1/cells/:namespace/agentspecs.
func (s *Server) listAgentSpecs(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	resources, err := s.store.List(ctx, "AgentSpec", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	specs := make([]*apiary.AgentSpec, 0, len(resources))
	for _, r := range resources {
		if spec, ok := r.(*apiary.AgentSpec); ok {
			specs = append(specs, spec)
		}
	}

	return c.JSON(http.StatusOK, specs)
}

// getAgentSpec handles GET /api/v1/cells/:namespace/agentspecs/:name.
func (s *Server) getAgentSpec(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	resource, err := s.store.Get(ctx, "AgentSpec", name, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// createAgentSpec handles POST /api/v1/cells/:namespace/agentspecs.
func (s *Server) createAgentSpec(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	var spec apiary.AgentSpec
	if err := c.Bind(&spec); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate required fields
	if len(spec.Spec.Runtime.Command) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "spec.runtime.command is required",
		})
	}
	if spec.Spec.Interface.Type == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "spec.interface.type is required",
		})
	}

	// Set namespace if not set
	if spec.ObjectMeta.Namespace == "" {
		spec.ObjectMeta.Namespace = namespace
	} else if spec.ObjectMeta.Namespace != namespace {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "namespace in URL does not match namespace in body",
		})
	}

	// Check resource quota before creating
	if err := s.checkResourceQuota(ctx, namespace, "AgentSpec"); err != nil {
		return c.JSON(http.StatusForbidden, errorResponse{
			Error:   "quota_exceeded",
			Message: err.Error(),
		})
	}

	if err := s.store.Create(ctx, &spec); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusCreated, spec)
}

// updateAgentSpec handles PUT /api/v1/cells/:namespace/agentspecs/:name.
func (s *Server) updateAgentSpec(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	var spec apiary.AgentSpec
	if err := c.Bind(&spec); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Validate namespace matches
	if err := validateResourceNamespace(spec.GetNamespace(), namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Validate required fields
	if len(spec.Spec.Runtime.Command) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "spec.runtime.command is required",
		})
	}
	if spec.Spec.Interface.Type == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "spec.interface.type is required",
		})
	}

	if spec.GetName() != name {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "name in URL does not match name in body",
		})
	}

	if err := s.store.Update(ctx, &spec); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, spec)
}

// deleteAgentSpec handles DELETE /api/v1/cells/:namespace/agentspecs/:name.
func (s *Server) deleteAgentSpec(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	if err := s.store.Delete(ctx, "AgentSpec", name, namespace); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// listHives handles GET /api/v1/cells/:namespace/hives.
func (s *Server) listHives(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	resources, err := s.store.List(ctx, "Hive", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	hives := make([]*apiary.Hive, 0, len(resources))
	for _, r := range resources {
		if hive, ok := r.(*apiary.Hive); ok {
			hives = append(hives, hive)
		}
	}

	return c.JSON(http.StatusOK, hives)
}

// getHive handles GET /api/v1/cells/:namespace/hives/:name.
func (s *Server) getHive(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	resource, err := s.store.Get(ctx, "Hive", name, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// createHive handles POST /api/v1/cells/:namespace/hives.
func (s *Server) createHive(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	// Check resource quota
	if err := s.checkResourceQuota(ctx, namespace, "Hive"); err != nil {
		return c.JSON(http.StatusForbidden, errorResponse{
			Error:   "quota_exceeded",
			Message: err.Error(),
		})
	}

	var hive apiary.Hive
	if err := c.Bind(&hive); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if hive.ObjectMeta.Namespace == "" {
		hive.ObjectMeta.Namespace = namespace
	} else if hive.ObjectMeta.Namespace != namespace {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "namespace in URL does not match namespace in body",
		})
	}

	if err := s.store.Create(ctx, &hive); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusCreated, hive)
}

// updateHive handles PUT /api/v1/cells/:namespace/hives/:name.
func (s *Server) updateHive(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	var hive apiary.Hive
	if err := c.Bind(&hive); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if hive.GetName() != name {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "name in URL does not match name in body",
		})
	}

	if hive.ObjectMeta.Namespace != namespace {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "namespace in URL does not match namespace in body",
		})
	}

	if err := s.store.Update(ctx, &hive); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, hive)
}

// deleteHive handles DELETE /api/v1/cells/:namespace/hives/:name.
func (s *Server) deleteHive(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	if err := s.store.Delete(ctx, "Hive", name, namespace); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// listSecrets handles GET /api/v1/cells/:namespace/secrets.
func (s *Server) listSecrets(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	resources, err := s.store.List(ctx, "Secret", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	secrets := make([]*apiary.Secret, 0, len(resources))
	for _, r := range resources {
		if secret, ok := r.(*apiary.Secret); ok {
			secrets = append(secrets, secret)
		}
	}

	return c.JSON(http.StatusOK, secrets)
}

// getSecret handles GET /api/v1/cells/:namespace/secrets/:name.
func (s *Server) getSecret(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	resource, err := s.store.Get(ctx, "Secret", name, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// createSecret handles POST /api/v1/cells/:namespace/secrets.
func (s *Server) createSecret(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	var secret apiary.Secret
	if err := c.Bind(&secret); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if secret.ObjectMeta.Namespace == "" {
		secret.ObjectMeta.Namespace = namespace
	} else if secret.ObjectMeta.Namespace != namespace {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "namespace in URL does not match namespace in body",
		})
	}

	// Validate secret data
	if secret.Data == nil || len(secret.Data) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "secret data cannot be empty",
		})
	}

	// Validate resource namespace
	if err := validateResourceNamespace(secret.ObjectMeta.Namespace, namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "namespace_mismatch",
			Message: err.Error(),
		})
	}

	if err := s.store.Create(ctx, &secret); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusCreated, secret)
}

// updateSecret handles PUT /api/v1/cells/:namespace/secrets/:name.
func (s *Server) updateSecret(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	var secret apiary.Secret
	if err := c.Bind(&secret); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	if secret.GetName() != name {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "name in URL does not match name in body",
		})
	}

	if secret.ObjectMeta.Namespace != namespace {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "namespace in URL does not match namespace in body",
		})
	}

	// Validate secret data
	if secret.Data == nil || len(secret.Data) == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: "secret data cannot be empty",
		})
	}

	// Validate resource namespace
	if err := validateResourceNamespace(secret.ObjectMeta.Namespace, namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "namespace_mismatch",
			Message: err.Error(),
		})
	}

	if err := s.store.Update(ctx, &secret); err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, secret)
}

// deleteSecret handles DELETE /api/v1/cells/:namespace/secrets/:name.
func (s *Server) deleteSecret(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	name := c.Param("name")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	if err := s.store.Delete(ctx, "Secret", name, namespace); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// listDrones handles GET /api/v1/cells/:namespace/drones.
func (s *Server) listDrones(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	resources, err := s.store.List(ctx, "Drone", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	drones := make([]*apiary.Drone, 0, len(resources))
	for _, r := range resources {
		if drone, ok := r.(*apiary.Drone); ok {
			drones = append(drones, drone)
		}
	}

	return c.JSON(http.StatusOK, drones)
}

// getDrone handles GET /api/v1/cells/:namespace/drones/:id.
func (s *Server) getDrone(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	id := c.Param("id")

	resource, err := s.store.Get(ctx, "Drone", id, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// deleteDrone handles DELETE /api/v1/cells/:namespace/drones/:id.
func (s *Server) deleteDrone(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	id := c.Param("id")

	if err := s.store.Delete(ctx, "Drone", id, namespace); err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// listSessions handles GET /api/v1/cells/:namespace/sessions.
func (s *Server) listSessions(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")

	resources, err := s.store.List(ctx, "Session", namespace, nil)
	if err != nil {
		return s.handleError(c, err)
	}

	sessions := make([]*apiary.Session, 0, len(resources))
	for _, r := range resources {
		if session, ok := r.(*apiary.Session); ok {
			sessions = append(sessions, session)
		}
	}

	return c.JSON(http.StatusOK, sessions)
}

// getSession handles GET /api/v1/cells/:namespace/sessions/:id.
func (s *Server) getSession(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	id := c.Param("id")

	resource, err := s.store.Get(ctx, "Session", id, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, resource)
}

// createSession handles POST /api/v1/cells/:namespace/hives/:hive/sessions.
func (s *Server) createSession(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// Parse session config from request body
	var req struct {
		TimeoutMinutes     int  `json:"timeoutMinutes,omitempty"`
		MaxDurationMinutes int  `json:"maxDurationMinutes,omitempty"`
		PersistOnTerminate bool `json:"persistOnTerminate,omitempty"`
		PersistPath        string `json:"persistPath,omitempty"`
		MaxMemoryMB        int  `json:"maxMemoryMB,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Create session with Hive name
	session, err := s.sessionMgr.CreateSession(ctx, session.SessionConfig{
		Namespace:          namespace,
		HiveName:           hiveName,
		TimeoutMinutes:     req.TimeoutMinutes,
		MaxDurationMinutes: req.MaxDurationMinutes,
		PersistOnTerminate: req.PersistOnTerminate,
		PersistPath:        req.PersistPath,
		MaxMemoryMB:        req.MaxMemoryMB,
	})
	if err != nil {
		return s.handleError(c, err)
	}

	// Set memory limit in Comb if specified
	if req.MaxMemoryMB > 0 && s.comb != nil {
		if store, ok := s.comb.(*comb.Store); ok {
			// Use namespace-prefixed sessionID for Comb isolation
			namespacedSessionID := fmt.Sprintf("%s:%s", namespace, session.GetUID())
			store.SetMemoryLimit(namespacedSessionID, req.MaxMemoryMB)
		}
	}

	return c.JSON(http.StatusCreated, session)
}

// sendMessage handles POST /api/v1/cells/:namespace/sessions/:id/messages.
func (s *Server) sendMessage(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	sessionID := c.Param("id")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Verify session exists and validate namespace
	sess, err := s.sessionMgr.GetSession(ctx, sessionID)
	if err != nil {
		return s.handleError(c, err)
	}

	// Validate that session belongs to the namespace
	if err := validateResourceNamespace(sess.GetNamespace(), namespace); err != nil {
		return c.JSON(http.StatusForbidden, errorResponse{
			Error:   "namespace_mismatch",
			Message: fmt.Sprintf("session namespace (%s) does not match URL namespace (%s)", sess.GetNamespace(), namespace),
		})
	}

	if sess.Status.Phase == apiary.SessionPhaseTerminated {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "session_terminated",
			Message: "Session is terminated",
		})
	}

	// Parse message
	var req struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		AgentID string `json:"agentId,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		})
	}

	// Update session activity
	if err := s.sessionMgr.UpdateActivity(ctx, sessionID); err != nil {
		return s.handleError(c, err)
	}

	// Append to conversation history if Comb is available
	if s.comb != nil {
		// Type assert to Store to access structured methods
		if store, ok := s.comb.(*comb.Store); ok {
			msg := comb.ConversationMessage{
				Type:    req.Type,
				Content: req.Content,
				AgentID: req.AgentID,
			}
			if err := store.AppendConversation(ctx, sessionID, msg); err != nil {
				s.logger.Warn("Failed to append message to conversation",
					zap.String("session", sessionID),
					zap.Error(err),
				)
			}
		}
	}

	// TODO: Route message to appropriate Drone
	// For now, just return success
	return c.JSON(http.StatusOK, map[string]string{
		"messageId": fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		"status":    "sent",
	})
}

// getSessionMemory handles GET /api/v1/cells/:namespace/sessions/:id/memory.
func (s *Server) getSessionMemory(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	sessionID := c.Param("id")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Verify session exists and validate namespace
	sess, err := s.sessionMgr.GetSession(ctx, sessionID)
	if err != nil {
		return s.handleError(c, err)
	}

	// Validate that session belongs to the namespace
	if err := validateResourceNamespace(sess.GetNamespace(), namespace); err != nil {
		return c.JSON(http.StatusForbidden, errorResponse{
			Error:   "namespace_mismatch",
			Message: fmt.Sprintf("session namespace (%s) does not match URL namespace (%s)", sess.GetNamespace(), namespace),
		})
	}

	if s.comb == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"keys": []string{},
		})
	}

	// Use namespace-prefixed sessionID for Comb isolation
	namespacedSessionID := fmt.Sprintf("%s:%s", namespace, sessionID)

	// Get all keys for the session
	keys, err := s.comb.Keys(ctx, namespacedSessionID, "*")
	if err != nil {
		return s.handleError(c, err)
	}

	// Build memory snapshot
	memory := make(map[string]interface{})
	for _, key := range keys {
		value, err := s.comb.Get(ctx, namespacedSessionID, key)
		if err != nil {
			continue
		}
		memory[key] = value
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"keys":   keys,
		"memory": memory,
	})
}

// deleteSession handles DELETE /api/v1/cells/:namespace/sessions/:id.
func (s *Server) deleteSession(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	sessionID := c.Param("id")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Verify session exists and validate namespace
	sess, err := s.sessionMgr.GetSession(ctx, sessionID)
	if err != nil {
		return s.handleError(c, err)
	}

	// Validate that session belongs to the namespace
	if err := validateResourceNamespace(sess.GetNamespace(), namespace); err != nil {
		return c.JSON(http.StatusForbidden, errorResponse{
			Error:   "namespace_mismatch",
			Message: fmt.Sprintf("session namespace (%s) does not match URL namespace (%s)", sess.GetNamespace(), namespace),
		})
	}

	// Terminate session via session manager
	if s.sessionMgr != nil {
		if err := s.sessionMgr.TerminateSession(ctx, sessionID); err != nil {
			return s.handleError(c, err)
		}
	}

	// Clear Comb data using namespace-prefixed sessionID
	if s.comb != nil {
		namespacedSessionID := fmt.Sprintf("%s:%s", namespace, sessionID)
		if err := s.comb.Clear(ctx, namespacedSessionID); err != nil {
			s.logger.Warn("Failed to clear Comb data",
				zap.String("session", sessionID),
				zap.Error(err),
			)
		}
	}

	return c.NoContent(http.StatusNoContent)
}

// listDLQMessages handles GET /api/v1/cells/:namespace/hives/:hive/dlq.
func (s *Server) listDLQMessages(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Check if DLQ manager is available
	if s.dlqMgr == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "DLQ service is not available",
		})
	}

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// List DLQ messages
	messages, err := s.dlqMgr.ListMessages(ctx, namespace, hiveName)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, messages)
}

// getDLQMessage handles GET /api/v1/cells/:namespace/hives/:hive/dlq/:messageId.
func (s *Server) getDLQMessage(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")
	messageID := c.Param("messageId")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Check if DLQ manager is available
	if s.dlqMgr == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "DLQ service is not available",
		})
	}

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// Get DLQ message
	msg, err := s.dlqMgr.GetMessage(ctx, namespace, hiveName, messageID)
	if err != nil {
		// Check if error is "not found" type
		if err.Error() == fmt.Sprintf("message not found: %s", messageID) ||
			err.Error() == fmt.Sprintf("DLQ not found for %s/%s", namespace, hiveName) {
			return c.JSON(http.StatusNotFound, errorResponse{
				Error:   "not_found",
				Message: err.Error(),
			})
		}
		return s.handleError(c, err)
	}

	return c.JSON(http.StatusOK, msg)
}

// replayDLQMessage handles POST /api/v1/cells/:namespace/hives/:hive/dlq/:messageId/replay.
func (s *Server) replayDLQMessage(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")
	messageID := c.Param("messageId")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Check if DLQ manager is available
	if s.dlqMgr == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "DLQ service is not available",
		})
	}

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// Get DLQ message
	dlqMsg, err := s.dlqMgr.GetMessage(ctx, namespace, hiveName, messageID)
	if err != nil {
		if err.Error() == fmt.Sprintf("message not found: %s", messageID) ||
			err.Error() == fmt.Sprintf("DLQ not found for %s/%s", namespace, hiveName) {
			return c.JSON(http.StatusNotFound, errorResponse{
				Error:   "not_found",
				Message: err.Error(),
			})
		}
		return s.handleError(c, err)
	}

	// For now, replay is a placeholder - actual replay would require message bus access
	// and knowledge of the original topic. This could be enhanced later.
	// For now, we just remove the message from DLQ (assuming it will be retried externally)
	// or we could add a Replay method to DLQ manager that publishes back to the bus.
	// Since we don't have bus access here, we'll just return success and let the caller
	// handle replay through other means.
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message": "Message retrieved for replay",
		"dlqMessage": dlqMsg,
		"note": "Actual replay requires message bus integration",
	})
}

// deleteDLQMessage handles DELETE /api/v1/cells/:namespace/hives/:hive/dlq/:messageId.
func (s *Server) deleteDLQMessage(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")
	messageID := c.Param("messageId")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Check if DLQ manager is available
	if s.dlqMgr == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "DLQ service is not available",
		})
	}

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// Delete DLQ message
	err = s.dlqMgr.DeleteMessage(ctx, namespace, hiveName, messageID)
	if err != nil {
		if err.Error() == fmt.Sprintf("message not found: %s", messageID) ||
			err.Error() == fmt.Sprintf("DLQ not found for %s/%s", namespace, hiveName) {
			return c.JSON(http.StatusNotFound, errorResponse{
				Error:   "not_found",
				Message: err.Error(),
			})
		}
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

// clearDLQ handles DELETE /api/v1/cells/:namespace/hives/:hive/dlq.
func (s *Server) clearDLQ(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.Param("namespace")
	hiveName := c.Param("hive")

	// Validate namespace
	if err := validateNamespace(namespace); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Error:   "invalid_namespace",
			Message: err.Error(),
		})
	}

	// Check if DLQ manager is available
	if s.dlqMgr == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{
			Error:   "service_unavailable",
			Message: "DLQ service is not available",
		})
	}

	// Verify Hive exists
	_, err := s.store.Get(ctx, "Hive", hiveName, namespace)
	if err != nil {
		return s.handleError(c, err)
	}

	// Clear DLQ
	err = s.dlqMgr.ClearMessages(ctx, namespace, hiveName)
	if err != nil {
		return s.handleError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

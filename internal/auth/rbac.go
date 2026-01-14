// Package auth provides role-based access control (RBAC) for Apiary.
package auth

import (
	"context"
	"fmt"
	"sync"
)

// Role represents a user role with specific permissions.
type Role string

const (
	// RoleAdmin has full access to all resources in a Cell.
	RoleAdmin Role = "admin"
	// RoleDeveloper can create, read, and update resources but cannot delete.
	RoleDeveloper Role = "developer"
	// RoleViewer can only read resources.
	RoleViewer Role = "viewer"
)

// Permission represents an action that can be performed on a resource.
type Permission string

const (
	// PermissionCreate allows creating new resources.
	PermissionCreate Permission = "create"
	// PermissionRead allows reading resources.
	PermissionRead Permission = "read"
	// PermissionUpdate allows updating existing resources.
	PermissionUpdate Permission = "update"
	// PermissionDelete allows deleting resources.
	PermissionDelete Permission = "delete"
)

// Principal represents an authenticated user/entity.
type Principal struct {
	ID   string
	Name string
}

// ContextKey is the key type for storing principal in context.
type contextKey string

const principalKey contextKey = "principal"

// WithPrincipal adds a principal to the context.
func WithPrincipal(ctx context.Context, principal *Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// PrincipalFromContext extracts the principal from the context.
func PrincipalFromContext(ctx context.Context) (*Principal, bool) {
	principal, ok := ctx.Value(principalKey).(*Principal)
	return principal, ok
}

// RBAC provides role-based access control.
type RBAC struct {
	// CellRoles maps Cell (namespace) to user ID to role
	cellRoles map[string]map[string]Role
	mu        sync.RWMutex
}

// NewRBAC creates a new RBAC instance.
func NewRBAC() *RBAC {
	return &RBAC{
		cellRoles: make(map[string]map[string]Role),
	}
}

// AssignRole assigns a role to a user in a specific Cell.
func (r *RBAC) AssignRole(cell, userID string, role Role) error {
	if cell == "" {
		return fmt.Errorf("cell name is required")
	}
	if userID == "" {
		return fmt.Errorf("user ID is required")
	}
	if !r.isValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cellRoles[cell] == nil {
		r.cellRoles[cell] = make(map[string]Role)
	}
	r.cellRoles[cell][userID] = role

	return nil
}

// GetRole returns the role of a user in a specific Cell.
func (r *RBAC) GetRole(cell, userID string) (Role, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userRoles, exists := r.cellRoles[cell]
	if !exists {
		return "", false
	}

	role, exists := userRoles[userID]
	return role, exists
}

// RemoveRole removes a role assignment for a user in a Cell.
func (r *RBAC) RemoveRole(cell, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	userRoles, exists := r.cellRoles[cell]
	if !exists {
		return nil // Nothing to remove
	}

	delete(userRoles, userID)
	if len(userRoles) == 0 {
		delete(r.cellRoles, cell)
	}

	return nil
}

// CheckPermission checks if a principal has the required permission for a resource in a Cell.
func (r *RBAC) CheckPermission(principal *Principal, cell string, permission Permission) bool {
	if principal == nil {
		return false // No principal, deny access
	}

	role, exists := r.GetRole(cell, principal.ID)
	if !exists {
		return false // No role assigned, deny access
	}

	return r.hasPermission(role, permission)
}

// hasPermission checks if a role has a specific permission.
func (r *RBAC) hasPermission(role Role, permission Permission) bool {
	switch role {
	case RoleAdmin:
		// Admin has all permissions
		return true
	case RoleDeveloper:
		// Developer has create, read, update (no delete)
		return permission == PermissionCreate || permission == PermissionRead || permission == PermissionUpdate
	case RoleViewer:
		// Viewer only has read permission
		return permission == PermissionRead
	default:
		return false
	}
}

// isValidRole checks if a role is valid.
func (r *RBAC) isValidRole(role Role) bool {
	switch role {
	case RoleAdmin, RoleDeveloper, RoleViewer:
		return true
	default:
		return false
	}
}

// ListCellRoles returns all role assignments for a Cell.
func (r *RBAC) ListCellRoles(cell string) map[string]Role {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userRoles, exists := r.cellRoles[cell]
	if !exists {
		return make(map[string]Role)
	}

	// Return a copy to prevent external modifications
	result := make(map[string]Role, len(userRoles))
	for userID, role := range userRoles {
		result[userID] = role
	}

	return result
}

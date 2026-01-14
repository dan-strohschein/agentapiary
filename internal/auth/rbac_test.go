package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRBAC_AssignRole(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)

	role, exists := rbac.GetRole("cell1", "user1")
	assert.True(t, exists)
	assert.Equal(t, RoleAdmin, role)
}

func TestRBAC_AssignRole_InvalidRole(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", Role("invalid"))
	assert.Error(t, err)
}

func TestRBAC_AssignRole_EmptyCell(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("", "user1", RoleAdmin)
	assert.Error(t, err)
}

func TestRBAC_AssignRole_EmptyUser(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "", RoleAdmin)
	assert.Error(t, err)
}

func TestRBAC_GetRole_NotExists(t *testing.T) {
	rbac := NewRBAC()

	_, exists := rbac.GetRole("cell1", "user1")
	assert.False(t, exists)
}

func TestRBAC_RemoveRole(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)

	err = rbac.RemoveRole("cell1", "user1")
	require.NoError(t, err)

	_, exists := rbac.GetRole("cell1", "user1")
	assert.False(t, exists)
}

func TestRBAC_RemoveRole_NotExists(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.RemoveRole("cell1", "user1")
	require.NoError(t, err) // Should not error if role doesn't exist
}

func TestRBAC_CheckPermission_Admin(t *testing.T) {
	rbac := NewRBAC()

	principal := &Principal{ID: "user1", Name: "User 1"}
	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)

	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionCreate))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionRead))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionUpdate))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionDelete))
}

func TestRBAC_CheckPermission_Developer(t *testing.T) {
	rbac := NewRBAC()

	principal := &Principal{ID: "user1", Name: "User 1"}
	err := rbac.AssignRole("cell1", "user1", RoleDeveloper)
	require.NoError(t, err)

	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionCreate))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionRead))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionUpdate))
	assert.False(t, rbac.CheckPermission(principal, "cell1", PermissionDelete))
}

func TestRBAC_CheckPermission_Viewer(t *testing.T) {
	rbac := NewRBAC()

	principal := &Principal{ID: "user1", Name: "User 1"}
	err := rbac.AssignRole("cell1", "user1", RoleViewer)
	require.NoError(t, err)

	assert.False(t, rbac.CheckPermission(principal, "cell1", PermissionCreate))
	assert.True(t, rbac.CheckPermission(principal, "cell1", PermissionRead))
	assert.False(t, rbac.CheckPermission(principal, "cell1", PermissionUpdate))
	assert.False(t, rbac.CheckPermission(principal, "cell1", PermissionDelete))
}

func TestRBAC_CheckPermission_NoRole(t *testing.T) {
	rbac := NewRBAC()

	principal := &Principal{ID: "user1", Name: "User 1"}
	// No role assigned

	assert.False(t, rbac.CheckPermission(principal, "cell1", PermissionRead))
}

func TestRBAC_CheckPermission_NilPrincipal(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)

	assert.False(t, rbac.CheckPermission(nil, "cell1", PermissionRead))
}

func TestRBAC_CheckPermission_DifferentCell(t *testing.T) {
	rbac := NewRBAC()

	principal := &Principal{ID: "user1", Name: "User 1"}
	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)

	// User has admin role in cell1, but not in cell2
	assert.False(t, rbac.CheckPermission(principal, "cell2", PermissionRead))
}

func TestRBAC_ListCellRoles(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)
	err = rbac.AssignRole("cell1", "user2", RoleDeveloper)
	require.NoError(t, err)
	err = rbac.AssignRole("cell1", "user3", RoleViewer)
	require.NoError(t, err)

	roles := rbac.ListCellRoles("cell1")
	assert.Len(t, roles, 3)
	assert.Equal(t, RoleAdmin, roles["user1"])
	assert.Equal(t, RoleDeveloper, roles["user2"])
	assert.Equal(t, RoleViewer, roles["user3"])
}

func TestRBAC_ListCellRoles_Empty(t *testing.T) {
	rbac := NewRBAC()

	roles := rbac.ListCellRoles("cell1")
	assert.Empty(t, roles)
}

func TestRBAC_ListCellRoles_Isolation(t *testing.T) {
	rbac := NewRBAC()

	err := rbac.AssignRole("cell1", "user1", RoleAdmin)
	require.NoError(t, err)
	err = rbac.AssignRole("cell2", "user1", RoleViewer)
	require.NoError(t, err)

	roles1 := rbac.ListCellRoles("cell1")
	assert.Len(t, roles1, 1)
	assert.Equal(t, RoleAdmin, roles1["user1"])

	roles2 := rbac.ListCellRoles("cell2")
	assert.Len(t, roles2, 1)
	assert.Equal(t, RoleViewer, roles2["user1"])
}

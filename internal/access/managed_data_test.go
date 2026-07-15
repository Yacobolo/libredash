package access

import (
	"slices"
	"testing"
)

func TestManagedDataPrivilegesAndDataDeployerRole(t *testing.T) {
	wantPrivileges := []Privilege{PrivilegeViewData, PrivilegeIngestData}
	known := KnownPrivileges()
	for _, privilege := range wantPrivileges {
		if !slices.Contains(known, privilege) {
			t.Fatalf("KnownPrivileges() = %#v, missing %s", known, privilege)
		}
	}
	for _, removed := range []Privilege{"ACTIVATE_DATA", "ACTIVATE_PUBLISH"} {
		if slices.Contains(known, removed) {
			t.Fatalf("KnownPrivileges() retains removed privilege %s", removed)
		}
	}

	var deployer *Role
	for _, role := range DefaultRoles() {
		if role.Name == RoleDataDeployer {
			copy := role
			deployer = &copy
			break
		}
	}
	if deployer == nil {
		t.Fatal("data_deployer role is missing")
	}
	if !slices.Equal(deployer.Privileges, wantPrivileges) {
		t.Fatalf("data_deployer privileges = %#v, want %#v", deployer.Privileges, wantPrivileges)
	}
}

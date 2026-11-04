package main

import "testing"

func TestPermissionMasks(t *testing.T) {
	tests := []struct {
		name      string
		granted   FilePermission
		requested FilePermission
		wantAll   bool
		wantAny   bool
	}{
		{name: "no permissions requested", wantAll: true},
		{name: "empty request with grants", granted: PermissionRead, wantAll: true},
		{name: "no permissions granted", requested: PermissionRead},
		{name: "single permission granted", granted: PermissionRead, requested: PermissionRead, wantAll: true, wantAny: true},
		{name: "single permission missing", granted: PermissionRead, requested: PermissionWrite},
		{name: "all requested permissions granted", granted: PermissionRead | PermissionWrite, requested: PermissionRead | PermissionWrite, wantAll: true, wantAny: true},
		{name: "only some requested permissions granted", granted: PermissionRead | PermissionWrite, requested: PermissionRead | PermissionExecute, wantAny: true},
		{name: "extra grants are allowed", granted: PermissionRead | PermissionWrite, requested: PermissionRead, wantAll: true, wantAny: true},
		{name: "unknown requested bit", granted: PermissionRead | PermissionWrite | PermissionExecute, requested: FilePermission(1 << 7)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAllPermissions(tt.granted, tt.requested); got != tt.wantAll {
				t.Errorf("hasAllPermissions(%03b, %03b) = %t, want %t", tt.granted, tt.requested, got, tt.wantAll)
			}
			if got := hasAnyPermission(tt.granted, tt.requested); got != tt.wantAny {
				t.Errorf("hasAnyPermission(%03b, %03b) = %t, want %t", tt.granted, tt.requested, got, tt.wantAny)
			}
		})
	}
}

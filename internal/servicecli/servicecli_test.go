package servicecli

import (
	"testing"

	"github.com/ggshr9/shuttle/service"
)

func TestParseScope(t *testing.T) {
	if parseScope("user") != service.ScopeUser {
		t.Error("user should map to ScopeUser")
	}
	if parseScope("system") != service.ScopeSystem {
		t.Error("system should map to ScopeSystem")
	}
	if parseScope("") != service.ScopeSystem {
		t.Error("empty should default to ScopeSystem")
	}
	if parseScope("bogus") != service.ScopeSystem {
		t.Error("unknown string should default to ScopeSystem")
	}
}

func TestScopeToString(t *testing.T) {
	if scopeToString(service.ScopeSystem) != "system" {
		t.Error("ScopeSystem → 'system'")
	}
	if scopeToString(service.ScopeUser) != "user" {
		t.Error("ScopeUser → 'user'")
	}
}

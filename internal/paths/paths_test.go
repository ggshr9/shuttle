package paths

import (
	"testing"
)

func TestResolveReturnsNonEmpty(t *testing.T) {
	for _, scope := range []Scope{ScopeSystem, ScopeUser} {
		p := Resolve(scope)
		if p.ConfigDir == "" {
			t.Errorf("scope=%v: ConfigDir is empty", scope)
		}
		if p.LogDir == "" {
			t.Errorf("scope=%v: LogDir is empty", scope)
		}
	}
}

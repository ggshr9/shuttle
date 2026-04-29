//go:build android || ios

package paths

// resolve returns empty paths on mobile. The mobile bindings inject
// platform-appropriate paths (sandboxed app-data dirs) directly into
// the runtime instead of routing through this package, so any caller
// that ends up here should treat the empty result as "use the
// host-side bridge value."
func resolve(scope Scope) Paths {
	return Paths{}
}

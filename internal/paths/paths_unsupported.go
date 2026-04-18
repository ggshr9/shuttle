//go:build !linux && !darwin && !windows

package paths

func resolve(scope Scope) Paths {
	return Paths{}
}

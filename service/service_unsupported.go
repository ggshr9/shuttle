//go:build !linux && !darwin && !windows

package service

func newManager(name string, scope Scope) (Manager, error) {
	return nil, ErrUnsupported
}

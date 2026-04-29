//go:build android || ios

package service

// On mobile, OS-level service managers (systemd / launchd / SCM)
// don't apply: the host app owns the lifecycle. Mirror the
// unsupported behaviour so any code path that reaches Manager
// fails closed instead of mis-targeting whatever the underlying
// OS happens to share with desktop (e.g. Android falling through
// to the Linux systemctl path).
func newManager(name string, scope Scope) (Manager, error) {
	return nil, ErrUnsupported
}

package mobile

// Callback is the interface that native (Android/iOS) code implements
// to receive engine events. All methods use simple types only for
// gomobile compatibility — no slices, maps, or complex Go types.
type Callback interface {
	// OnStatusChange is called when the engine state changes.
	// state is one of: "stopped", "starting", "running", "stopping".
	OnStatusChange(state string)

	// OnNetworkChange is called when a WiFi/cellular network change is detected.
	OnNetworkChange()

	// OnError is called on non-fatal errors. code is one of the Err* constants.
	OnError(code int, message string)

	// OnSpeedUpdate is called every second with current upload/download bytes per second.
	OnSpeedUpdate(upload, download int64)
}

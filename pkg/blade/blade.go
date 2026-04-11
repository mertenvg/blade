package blade

// Deprecated: Done is a no-op retained for backward compatibility.
// It previously cleaned up a PID file used for process tracking.
// Blade now uses process group signaling instead. It is safe to
// remove calls to Done and the associated import.
func Done() {}

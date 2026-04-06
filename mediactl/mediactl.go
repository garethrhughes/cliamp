// Package mediactl defines the interface for platform media controller
// integrations (MPRIS on Linux, media keys on macOS, etc.) and provides
// a registry so controllers can self-register via init().
package mediactl

// TrackInfo carries metadata for the currently playing track.
type TrackInfo struct {
	Title       string
	Artist      string
	Album       string
	Genre       string
	TrackNumber int
	URL         string
	LengthUs    int64 // microseconds
}

// Controller is implemented by platform-specific media integrations
// (e.g. MPRIS on Linux, MPRemoteCommandCenter on macOS).
type Controller interface {
	// Update refreshes the controller with the current playback state.
	// status is "Playing", "Paused", or "Stopped".
	Update(status string, track TrackInfo, volumeDB float64, positionUs int64, canSeek bool)

	// EmitSeeked notifies the controller that a seek just occurred.
	// positionUs is the new absolute position in microseconds.
	EmitSeeked(positionUs int64)

	// Close releases resources held by the controller.
	Close()
}

// MainLoopRunner is optionally implemented by controllers that need the
// main OS thread (e.g. macOS NSRunLoop for media key events).
type MainLoopRunner interface {
	RunMainLoop(done <-chan struct{})
}

// RunMainLoop checks if any controller needs the main thread and runs
// its main loop, blocking until done is closed. If no controller needs
// the main thread, it simply blocks on done.
func RunMainLoop(controllers []Controller, done <-chan struct{}) {
	for _, c := range controllers {
		if r, ok := c.(MainLoopRunner); ok {
			r.RunMainLoop(done)
			return
		}
	}
	<-done
}

// InitMsg is sent to the Bubbletea model after controllers are created.
type InitMsg struct{ Controllers []Controller }

// InitFunc creates a Controller. send injects messages into the
// Bubbletea event loop (typically prog.Send).
type InitFunc func(send func(interface{})) (Controller, error)

var registry []InitFunc

// Register adds a controller factory to the registry. Platform-specific
// packages call this from init().
func Register(fn InitFunc) {
	registry = append(registry, fn)
}

// InitAll instantiates all registered controllers. Errors from individual
// controllers are silently ignored (the controller is simply skipped).
func InitAll(send func(interface{})) []Controller {
	var controllers []Controller
	for _, fn := range registry {
		if c, err := fn(send); err == nil && c != nil {
			controllers = append(controllers, c)
		}
	}
	return controllers
}

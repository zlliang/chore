package runner

import (
	"os"
	"time"
)

// Event is the interface implemented by all runner events.
type Event interface {
	eventMarker()
}

type EventTaskStarted struct{ Task string }
type EventTaskSkipped struct{ Task string }

type EventTaskOutput struct {
	Task    string
	Text    string
	Replace bool // Replace the last output line instead of appending.
}

type EventTaskInteractive struct {
	Task string
	PTY  *os.File
}

type EventTaskCompleted struct {
	Task     string
	Duration time.Duration
}

type EventTaskFailed struct {
	Task     string
	Err      error
	Duration time.Duration
}

type EventRunDone struct{}

func (EventTaskStarted) eventMarker()     {}
func (EventTaskSkipped) eventMarker()     {}
func (EventTaskOutput) eventMarker()      {}
func (EventTaskInteractive) eventMarker() {}
func (EventTaskCompleted) eventMarker()   {}
func (EventTaskFailed) eventMarker()      {}
func (EventRunDone) eventMarker()         {}

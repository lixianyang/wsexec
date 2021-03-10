package wsexec

import (
	"errors"
)

const (
	EndOfTransmission = "\u0004"
	Escape            = '\u001b'
)

var (
	ErrTerminalSizeMonitorStopped = errors.New("terminal size monitor has been stopped")
	ErrUnexpectedMessageType      = errors.New("received unexpected message type")
)


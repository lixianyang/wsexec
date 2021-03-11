package wsexec

import (
	"fmt"
	"io"
	"log"
)

type Logger interface {
	Printfln(format string, a ...interface{})
	Println(a ...interface{})
}

type discardLogger struct{}

func (l discardLogger) Printfln(format string, a ...interface{}) {}
func (l discardLogger) Println(a ...interface{})                 {}

type stdLogger struct {
	logger *log.Logger
}

func (r *stdLogger) Printfln(format string, a ...interface{}) {
	r.logger.Output(2, fmt.Sprintf(format, a...))
}

func (r *stdLogger) Println(a ...interface{}) {
	r.logger.Output(2, fmt.Sprint(a...))
}

func NewLogger(writer io.Writer) Logger {
	logger := log.New(writer, "[wsexec]", log.Ltime|log.Lmicroseconds|log.Lshortfile)
	return &stdLogger{
		logger: logger,
	}
}

package wsexec

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

type Server struct {
	conn         *websocket.Conn
	resizeChan   chan remotecommand.TerminalSize
	doneChan     chan error
	ticker       *time.Ticker
	pingInterval time.Duration
	pingTimeout  time.Duration
	closeTimeout time.Duration
	sync.Mutex   // write lock
	logger       Logger
	debugInput   io.Writer
	debugOutput  io.Writer
}

type ServerOption func(s *Server)

func WithServerPingInterval(d time.Duration) ServerOption {
	return func(s *Server) {
		s.pingInterval = d
	}
}

func WithServerPingTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.pingTimeout = d
	}
}

func WithServerCloseTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.closeTimeout = d
	}
}

func WithServerLogger(logger Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

func WithServerDebugInput(writer io.Writer) ServerOption {
	return func(s *Server) {
		s.debugInput = writer
	}
}

func WithServerDebugOutput(writer io.Writer) ServerOption {
	return func(s *Server) {
		s.debugOutput = writer
	}
}

func NewServer(conn *websocket.Conn, options ...ServerOption) *Server {
	defaultPingInterval := 10 * time.Second
	defaultPingTimeout := 5 * time.Second
	defaultCloseTimeout := 5 * time.Second
	defaultLogger := discardLogger{}

	s := &Server{
		conn:         conn,
		resizeChan:   make(chan remotecommand.TerminalSize, 1),
		doneChan:     make(chan error, 2),
		pingInterval: defaultPingInterval,
		pingTimeout:  defaultPingTimeout,
		closeTimeout: defaultCloseTimeout,
		logger:       defaultLogger,
	}

	for _, opt := range options {
		opt(s)
	}

	s.ticker = time.NewTicker(s.pingInterval)

	return s
}

func (s *Server) Close(err error) {
	s.logger.Println("close with err=", err)
	s.doneChan <- err
}

func (s *Server) Keepalive() {
	defer s.conn.Close()

	var err error
	for {
		select {
		case <-s.ticker.C:
			s.logger.Println("keepalive goroutine send ping message")
			if err = s.Ping(); err != nil {
				s.logger.Println("keepalive goroutine returned with ping err ", err)
				return
			}
		case err = <-s.doneChan:
			s.logger.Println("keepalive goroutine returned with done err=", err)
			s.sendCloseMessageIfNeeded(err)
			return
		}
	}
}

func (s *Server) sendCloseMessageIfNeeded(err error) {
	if err == nil {
		return
	}

	if e, ok := err.(*websocket.CloseError); ok {
		s.logger.Printfln("needn't send close message with websocket close error code %d message %s", e.Code, e.Text)
		return
	}

	s.logger.Println("send close message with err ", err)
	closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, err.Error())
	s.Lock()
	defer s.Unlock()
	if e := s.conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(s.closeTimeout)); e != nil {
		s.logger.Println("send close message with write control err ", e)
	}

	return
}

func (s *Server) Ping() error {
	s.Lock()
	defer s.Unlock()

	return s.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(s.pingTimeout))
}

func (s *Server) Read(p []byte) (n int, err error) {
	var t int
	var data []byte
	for {
		t, data, err = s.conn.ReadMessage()
		if err != nil {
			break
		}

		if isTerminalSizeChangeType(t) {
			s.logger.Println("read terminal size change message: ", string(data))
			size := remotecommand.TerminalSize{}
			if err = unmarshalTerminalSize(data, &size); err != nil {
				s.logger.Println("unmarshal terminal size message err ", err)
				break
			}
			s.resizeChan <- size
			continue
		}

		if isDataType(t) {
			s.recordInput(data)
			n = copy(p, data)
			break
		}

		err = ErrUnexpectedMessageType
	}

	if err == nil || err == io.EOF {
		return
	}

	if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		s.logger.Println("silence websocket normal close")
		err = io.EOF
	} else if errors.Is(err, net.ErrClosed) {
		s.logger.Println("silence ", net.ErrClosed)
		err = io.EOF
	} else {
		if e, ok := err.(*websocket.CloseError); ok {
			s.logger.Printfln("silence websocket close error with code: %d message: %s", e.Code, e.Text)
			err = io.EOF
		}
		s.logger.Println("cleanup remote session with ", EndOfTransmission, "(EOT)")
		n = copy(p, EndOfTransmission)
	}

	if err == io.EOF {
		s.doneChan <- nil
	} else {
		s.doneChan <- err
	}

	return
}

func (s *Server) Write(p []byte) (n int, err error) {
	s.Lock()
	defer s.Unlock()

	if err = s.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		s.logger.Println("write err ", err)
	}

	s.recordOutput(p)
	return len(p), err
}

func (s *Server) Next() *remotecommand.TerminalSize {
	select {
	case size := <-s.resizeChan:
		return &size
	}
}

func (s *Server) recordInput(data []byte) {
	if s.debugInput != nil {
		_, _ = s.debugInput.Write([]byte(fmt.Sprintf("%+q\n", data)))
	}
}

func (s *Server) recordOutput(data []byte) {
	if s.debugOutput != nil {
		_, _ = s.debugOutput.Write(data)
	}
}

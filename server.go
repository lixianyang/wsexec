package wsexec

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
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

func NewServer(conn *websocket.Conn, options ...ServerOption) *Server {
	defaultPingInterval := 10 * time.Second
	defaultPingTimeout := 5 * time.Second
	defaultCloseTimeout := 5 * time.Second

	s := &Server{
		conn:         conn,
		resizeChan:   make(chan remotecommand.TerminalSize, 1),
		doneChan:     make(chan error, 2),
		pingInterval: defaultPingInterval,
		pingTimeout:  defaultPingTimeout,
		closeTimeout: defaultCloseTimeout,
	}

	for _, opt := range options {
		opt(s)
	}

	s.ticker = time.NewTicker(s.pingInterval)

	return s
}

func (ws *Server) Close(err error) {
	klog.V(2).Infoln("close with err=", err)
	ws.doneChan <- err
}

func (ws *Server) Keepalive() {
	defer ws.conn.Close()

	var err error
	for {
		select {
		case <-ws.ticker.C:
			klog.V(2).Infoln("keepalive goroutine send ping message")
			if err = ws.Ping(); err != nil {
				klog.V(2).Infoln("keepalive goroutine returned with ping err ", err)
				return
			}
		case err = <-ws.doneChan:
			klog.V(2).Infoln("keepalive goroutine returned with done err=", err)
			ws.sendCloseMessageIfNeeded(err)
			return
		}
	}
}

func (ws *Server) sendCloseMessageIfNeeded(err error) {
	if err == nil {
		return
	}

	if e, ok := err.(*websocket.CloseError); ok {
		klog.V(2).Infoln("needn't send close message with websocket close error code %d message %s", e.Code, e.Text)
		return
	}

	klog.V(2).Infoln("send close message with err ", err)
	closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, err.Error())
	ws.Lock()
	defer ws.Unlock()
	if e := ws.conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(ws.closeTimeout)); e != nil {
		klog.V(2).Infoln("send close message with write control err ", e)
	}

	return
}

func (ws *Server) Ping() error {
	ws.Lock()
	defer ws.Unlock()

	return ws.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(ws.pingTimeout))
}

func (ws *Server) Read(p []byte) (n int, err error) {
	var t int
	var data []byte
	for {
		t, data, err = ws.conn.ReadMessage()
		if err != nil {
			break
		}

		if isTerminalSizeChangeType(t) {
			klog.V(2).Infoln("read terminal size change message: %s", string(data))
			size := remotecommand.TerminalSize{}
			if err = unmarshalTerminalSize(data, &size); err != nil {
				klog.V(2).Infoln("unmarshal terminal size message err ", err)
				break
			}
			ws.resizeChan <- size
			continue
		}

		if isDataType(t) {
			klog.V(0).Infof("read data: %+q", data)
			n = copy(p, data)
			break
		}

		err = ErrUnexpectedMessageType
	}

	if err == nil || err == io.EOF {
		return
	}

	if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		klog.V(2).Infoln("silence websocket normal close")
		err = io.EOF
	} else if errors.Is(err, net.ErrClosed) {
		klog.V(2).Infoln("silence ", net.ErrClosed)
		err = io.EOF
	} else {
		if e, ok := err.(*websocket.CloseError); ok {
			klog.V(2).Infoln("silence websocket close error with code: %d message: %s", e.Code, e.Text)
			err = io.EOF
		}
		klog.V(2).Infoln("cleanup remote session with ", EndOfTransmission, "(EOT)")
		n = copy(p, EndOfTransmission)
	}

	if err == io.EOF {
		ws.doneChan <- nil
	} else {
		ws.doneChan <- err
	}

	return
}

func (ws *Server) Write(p []byte) (n int, err error) {
	ws.Lock()
	defer ws.Unlock()

	if err = ws.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		klog.V(2).Infoln("write err ", err)
	}

	klog.V(0).Infof("write data: %+q", p)
	return len(p), err
}

func (ws *Server) Next() *remotecommand.TerminalSize {
	select {
	case size := <-ws.resizeChan:
		return &size
	}
}

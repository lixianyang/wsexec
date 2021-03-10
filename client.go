package wsexec

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gorilla/websocket"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/util/term"
)

type Client struct {
	conn      *websocket.Conn
	errChan   chan error
	writeChan chan message
	tty       term.TTY
}

type ClientOption func(cli *Client)

func WithClientTTY(tty term.TTY) ClientOption {
	return func(cli *Client) {
		cli.tty = tty
	}
}

func NewClient(conn *websocket.Conn, options ...ClientOption) *Client {
	defaultTTY := term.TTY{In: os.Stdin, Out: os.Stdout, Raw: true}

	client := &Client{
		conn:      conn,
		errChan:   make(chan error, 1),
		writeChan: make(chan message, 1),
		tty:       defaultTTY,
	}

	for _, opt := range options {
		opt(client)
	}

	return client
}

func (ws *Client) Run() error {
	fn := func() error {
		go ws.send()
		go ws.monitorTerminalSize()
		go ws.flushOut(ws.tty.Out)
		go ws.scanInput(ws.tty.In)

		err := <-ws.errChan
		klog.V(2).Infoln("received error %v", err)
		var closeError *websocket.CloseError
		if errors.As(err, &closeError) {
			klog.V(2).Infoln("silence websocket close error code: %d message: %s", closeError.Code, closeError.Text)
			err = nil
		}
		return err
	}

	return ws.tty.Safe(fn)
}

// monitor terminal size, and send change size command
func (ws *Client) monitorTerminalSize() {
	sizeQueue := ws.tty.MonitorSize(ws.tty.GetSize())
	if sizeQueue == nil {
		klog.V(2).Infoln("there's no TTY present.")
		return
	}

	klog.V(2).Infoln("monitor terminal size goroutine start")

	for {
		size := sizeQueue.Next()
		if size == nil {
			klog.V(2).Infoln("monitor terminal size goroutine returned with err ", ErrTerminalSizeMonitorStopped)
			ws.errChan <- ErrTerminalSizeMonitorStopped
			return
		}

		data, err := marshalTerminalSize(*size)
		if err != nil {
			klog.V(2).Infoln("monitor terminal size goroutine returned with err ", err)
			ws.errChan <- fmt.Errorf("terminal size marshal %w", err)
			return
		}

		klog.V(2).Infoln("send terminal size change message %s")
		ws.writeChan <- newTerminalSizeChangeMessage(data)
	}
}

func (ws *Client) flushOut(writer io.Writer) {
	klog.V(2).Infoln("output flush goroutine start")

	for {
		_, reader, err := ws.conn.NextReader()
		if err != nil {
			klog.V(2).Infoln("output flush goroutine returned with next reader err ", err)
			ws.errChan <- fmt.Errorf("read data from connection %w", err)
			return
		}
		if _, err = io.Copy(writer, reader); err != nil {
			klog.V(2).Infoln("output flush goroutine returned with io copy err ", err)
			ws.errChan <- fmt.Errorf("copy data from connection to output %w", err)
			return
		}
	}
}

func (ws *Client) send() {
	klog.V(2).Infoln("send goroutine start")

	var err error
	for msg := range ws.writeChan {
		if err = ws.conn.WriteMessage(int(msg.Type), msg.Data); err != nil {
			klog.V(2).Infoln("send goroutine returned with write message err ", err)
			ws.errChan <- fmt.Errorf("write data to connection %w", err)
			return
		}
	}

	klog.V(2).Infoln("send goroutine returned")
}

func (ws *Client) scanInput(reader io.Reader) {
	klog.V(2).Infoln("scan input goroutine start")

	br := bufio.NewReader(reader)
	for {
		rn, _, err := br.ReadRune()
		if err != nil {
			klog.V(2).Infoln("scan input goroutine returned with read rune err ", err)
			ws.errChan <- fmt.Errorf("read data from input %w", err)
			return
		}

		bytes := []byte(string(rn))

		if rn == Escape {
			p := make([]byte, 128)
			n, err := br.Read(p)
			if err != nil {
				if err != io.EOF {
					klog.V(2).Infoln("scan input goroutine returned with read escaped characters err ", err)
					ws.errChan <- fmt.Errorf("read escaped character from input %w", err)
					return
				}
			} else {
				bytes = append(bytes, p[:n]...)
			}
		}

		klog.V(0).Infof("scan input: %+q", bytes)
		ws.writeChan <- newDataMessage(bytes)
	}
}

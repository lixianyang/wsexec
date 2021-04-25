package wsexec

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/gorilla/websocket"
	dockerterm "github.com/moby/term"
	"k8s.io/kubectl/pkg/util/term"
)

type Client struct {
	conn       *websocket.Conn
	errChan    chan error
	writeChan  chan message
	tty        term.TTY
	debugInput io.Writer
	logger     Logger
}

type ClientOption func(cli *Client)

func WithClientTTY(tty term.TTY) ClientOption {
	return func(cli *Client) {
		cli.tty = tty
	}
}

func WithClientLogger(logger Logger) ClientOption {
	return func(cli *Client) {
		cli.logger = logger
	}
}

func WithClientDebugInput(writer io.Writer) ClientOption {
	return func(cli *Client) {
		cli.debugInput = writer
	}
}

func NewClient(conn *websocket.Conn, options ...ClientOption) *Client {
	in, out, _ := dockerterm.StdStreams()
	defaultTTY := term.TTY{In: in, Out: out, Raw: true}
	defaultLogger := discardLogger{}

	client := &Client{
		conn:      conn,
		errChan:   make(chan error, 1),
		writeChan: make(chan message, 1),
		tty:       defaultTTY,
		logger:    defaultLogger,
	}

	for _, opt := range options {
		opt(client)
	}

	return client
}

func (cli *Client) Run() error {
	fn := func() error {
		go cli.send()
		go cli.monitorTerminalSize()
		go cli.flushOut(cli.tty.Out)
		go cli.scanInput(cli.tty.In)

		err := <-cli.errChan
		cli.logger.Printfln("received error %s", err)
		var closeError *websocket.CloseError
		if errors.As(err, &closeError) {
			cli.logger.Printfln("silence websocket close error code: %d message: %s", closeError.Code, closeError.Text)
			err = nil
		}
		return err
	}

	return cli.tty.Safe(fn)
}

// monitor terminal size, and send change size command
func (cli *Client) monitorTerminalSize() {
	sizeQueue := cli.tty.MonitorSize(cli.tty.GetSize())
	if sizeQueue == nil {
		cli.logger.Println("there's no TTY present.")
		return
	}

	cli.logger.Println("monitor terminal size goroutine start")

	for {
		size := sizeQueue.Next()
		if size == nil {
			cli.logger.Println("monitor terminal size goroutine returned with err ", ErrTerminalSizeMonitorStopped)
			cli.errChan <- ErrTerminalSizeMonitorStopped
			return
		}

		data, err := marshalTerminalSize(*size)
		if err != nil {
			cli.logger.Println("monitor terminal size goroutine returned with err ", err)
			cli.errChan <- fmt.Errorf("terminal size marshal %w", err)
			return
		}

		cli.logger.Printfln("send terminal size change message %s", string(data))
		cli.writeChan <- newTerminalSizeChangeMessage(data)
	}
}

func (cli *Client) flushOut(writer io.Writer) {
	cli.logger.Println("output flush goroutine start")

	for {
		_, reader, err := cli.conn.NextReader()
		if err != nil {
			cli.logger.Println("output flush goroutine returned with next reader err ", err)
			cli.errChan <- fmt.Errorf("read data from connection %w", err)
			return
		}
		if _, err = io.Copy(writer, reader); err != nil {
			cli.logger.Println("output flush goroutine returned with io copy err ", err)
			cli.errChan <- fmt.Errorf("copy data from connection to output %w", err)
			return
		}
	}
}

func (cli *Client) send() {
	cli.logger.Println("send goroutine start")

	var err error
	for msg := range cli.writeChan {
		if err = cli.conn.WriteMessage(int(msg.Type), msg.Data); err != nil {
			cli.logger.Println("send goroutine returned with write message err ", err)
			cli.errChan <- fmt.Errorf("write data to connection %w", err)
			return
		}
	}

	cli.logger.Println("send goroutine returned")
}

func (cli *Client) scanInput(reader io.Reader) {
	cli.logger.Println("scan input goroutine start")

	br := bufio.NewReader(reader)
	for {
		rn, _, err := br.ReadRune()
		if err != nil {
			cli.logger.Println("scan input goroutine returned with read rune err ", err)
			cli.errChan <- fmt.Errorf("read data from input %w", err)
			return
		}

		bytes := []byte(string(rn))

		if rn == Escape {
			p := make([]byte, 128)
			n, err := br.Read(p)
			if err != nil {
				if err != io.EOF {
					cli.logger.Println("scan input goroutine returned with read escaped characters err ", err)
					cli.errChan <- fmt.Errorf("read escaped character from input %w", err)
					return
				}
			} else {
				bytes = append(bytes, p[:n]...)
			}
		}

		cli.recordInput(bytes)
		cli.writeChan <- newDataMessage(bytes)
	}
}

func (cli *Client) recordInput(data []byte) {
	if cli.debugInput != nil {
		_, _ = cli.debugInput.Write([]byte(fmt.Sprintf("%+q\n", data)))
	}
}

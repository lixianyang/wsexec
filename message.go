package wsexec

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"k8s.io/client-go/tools/remotecommand"
)

func marshalTerminalSize(size remotecommand.TerminalSize) ([]byte, error) {
	return json.Marshal(size)
}

func unmarshalTerminalSize(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

type payloadType int

var (
	terminalSizeChangeType payloadType = websocket.TextMessage
	dataType               payloadType = websocket.BinaryMessage
)

type message struct {
	Type payloadType
	Data []byte
}

func newTerminalSizeChangeMessage(data []byte) message {
	return message{
		Type: terminalSizeChangeType,
		Data: data,
	}
}

func newDataMessage(data []byte) message {
	return message{
		Type: dataType,
		Data: data,
	}
}

func isDataType(t int) bool {
	return t == int(dataType)
}

func isTerminalSizeChangeType(t int) bool {
	return t == int(terminalSizeChangeType)
}

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/lixianyang/wsexec"
)

func main() {
	namespace := "default"
	pod := "your-pod-name"
	command := "your-command"
	query := url.Values{}
	query.Set("namespace", namespace)
	query.Set("pod", pod)
	query.Set("command", command)
	u, _ := url.Parse("ws://127.0.0.1:8080/exec")
	u.RawQuery = query.Encode()

	headers := http.Header{}

	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		fmt.Println(err)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		} else {
			fmt.Println(resp.StatusCode, ":", string(body))
		}
		return
	}

	client := wsexec.NewClient(conn)
	if err = client.Run(); err != nil {
		panic(err)
	}
}

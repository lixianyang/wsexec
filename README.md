# Introduction

websocket client and server lib for kubernetes exec command.

# Install

```shell
go get github.com/lixianyang/wsexec
```

# Example

## server

Change kubeconfig load way if needed.

See [examples](examples/server/main.go)

```
cd example/server
go mod tidy
go run main.go
```

## client

Change namespace, pod and command before run main.go

See [examples](examples/client/main.go)

```
cd example/client
go mod tidy
go run main.go
```

# Introduction

websocket client and server lib for kubernetes exec command.

# Install

```shell
go get github.com/lixianyang/wsexec
```

# Example

## server

Use https://github.com/kubernetes/client-go/blob/master/tools/clientcmd/doc.go load kubeconfig.

```shell
cd example/server
go mod tidy
go run main.go
```

## client

Change namespace, pod and command before run main.go

```shell
cd example/client
go mod tidy
go run main.go
```

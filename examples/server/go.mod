module github.com/lixianyang/wsexec/examples/server

go 1.16

require (
	github.com/gorilla/websocket v1.4.2
	github.com/lixianyang/wsexec v0.0.0-20210316062303-7035ad2669e7
	k8s.io/api v0.18.16
	k8s.io/apimachinery v0.18.16
	k8s.io/client-go v0.18.16
)

replace k8s.io/kubectl => k8s.io/kubectl v0.18.16

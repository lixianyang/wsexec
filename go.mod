module github.com/lixianyang/wsexec

go 1.16

require (
	github.com/gorilla/websocket v1.4.2
	github.com/moby/term v0.0.0-20201216013528-df9cb8a40635
	k8s.io/client-go v0.18.16
	k8s.io/kubectl v0.0.0
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.18.16
	k8s.io/kubectl => k8s.io/kubectl v0.18.16
)

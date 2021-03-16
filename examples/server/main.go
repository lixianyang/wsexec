package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/lixianyang/wsexec"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	restConfig *rest.Config
	restClient *rest.RESTClient
	clientSet  *kubernetes.Clientset
)

func init() {
	// see https://github.com/kubernetes/client-go/blob/master/tools/clientcmd/doc.go
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here
	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		panic(err)
	}

	config.APIPath = "/api"
	config.GroupVersion = &corev1.SchemeGroupVersion
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	restClient, err = rest.RESTClientFor(config)
	if err != nil {
		panic(err)
	}
	clientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	restConfig = config
}

func handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	namespace := q.Get("namespace")
	if namespace == "" {
		namespace = "default"
	}
	podName := q.Get("pod")
	if podName == "" {
		w.WriteHeader(400)
		fmt.Fprint(w, "miss pod query parameter")
		return
	}
	containerName := q.Get("container")
	command := q.Get("command")
	if command == "" {
		w.WriteHeader(400)
		fmt.Fprint(w, "miss command query parameter")
		return
	}

	pod, err := clientSet.CoreV1().Pods(namespace).Get(r.Context(), podName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			w.WriteHeader(400)
			fmt.Fprintf(w, "can't found pod %s", podName)
			return
		} else {
			w.WriteHeader(500)
			fmt.Fprintf(w, "get pod error %s", err)
			return
		}
	}
	if pod.Status.Phase != corev1.PodRunning {
		w.WriteHeader(400)
		fmt.Fprintf(w, "only running pod can exec, pod %s in %s phase now", pod.Name, pod.Status.Phase)
		return
	}
	if containerName == "" {
		containerName, err = getContainerNameForExec(pod)
		if err != nil {
			w.WriteHeader(400)
			fmt.Fprint(w, err.Error())
			return
		}
	}

	req := restClient.Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec")
	execOption := &corev1.PodExecOptions{
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
		Container: containerName,
		Command:   []string{command},
	}
	req.VersionedParams(execOption, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "can't upgrade to spdy %s", err)
		return
	}

	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "can't upgrade to websocket %s", err)
		return
	}

	s := wsexec.NewServer(conn)
	go s.Keepalive()

	streamOption := remotecommand.StreamOptions{
		Stdin:             s,
		Stdout:            s,
		Stderr:            s,
		Tty:               true,
		TerminalSizeQueue: s,
	}
	if err = exec.Stream(streamOption); err != nil {
		fmt.Println("stream returned with ", err)
	}

	s.Close(err)
}

func main() {
	http.HandleFunc("/exec", handler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func getContainerNameForExec(pod *corev1.Pod) (name string, err error) {
	containers := make(map[string]bool)
	names := make([]string, 0)
	for _, c := range pod.Spec.Containers {
		containers[c.Name] = true
		names = append(names, c.Name)
	}
	const sideCarContainerName = "istio-proxy"
	switch len(containers) {
	case 1:
		name = names[0]
	case 2:
		if containers[sideCarContainerName] {
			if names[0] == sideCarContainerName {
				name = names[1]
			} else {
				name = names[0]
			}
		} else {
			err = fmt.Errorf("a container name must be specified for pod %s, choose one of: [%s]", pod.Name, strings.Join(names, " "))
		}
	default:
		err = fmt.Errorf("a container name must be specified for pod %s, choose one of: [%s]", pod.Name, strings.Join(names, " "))
	}
	return
}

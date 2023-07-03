package operatorkclient

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/Noksa/operator-home/internal/operatorcache"
	"go.uber.org/multierr"
	"io"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"sync"
	"time"
)

const debug = false

var clientSet *kubernetes.Clientset
var config *rest.Config
var m sync.Mutex

func GetClientConfig() *rest.Config {
	if config != nil {
		return config
	}
	config = ctrl.GetConfigOrDie()
	return config
}

func GetPodContainerLogs(ctx context.Context, namespace string, podName string, containerName string, sinceTime *metav1.Time) (string, error) {
	podLogsRequest := clientSet.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		SinceTime: sinceTime,
	})
	logStream, err := podLogsRequest.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer logStream.Close()
	builder := strings.Builder{}
	reader := bufio.NewScanner(logStream)
	var line string
	t := time.Now()
	for time.Since(t) <= time.Minute*1 {
		select {
		case <-ctx.Done():
			return builder.String(), nil
		default:
			for reader.Scan() {
				line = reader.Text()
				builder.WriteString(fmt.Sprintf("%v\n", line))
			}
			return builder.String(), nil
		}
	}
	return "", fmt.Errorf("timed out in GetPodContainerLogs")
}

func RunCommandInPodWithContextAndTimeout(ctx context.Context, timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, error) {
	if timeout < time.Millisecond*1 {
		timeout = time.Millisecond * 1
	}
	myCtx, cancel := context.WithTimeout(ctx, timeout)
	var mErr error
	var result string
	go func() {
		defer cancel()
		objName := fmt.Sprintf("%v-%v-%v", namespace, podName, containerName)
		m.Lock()
		mutexForObject, found := operatorcache.Get[*sync.Mutex](objName)
		if !found {
			mutexForObject = &sync.Mutex{}
			operatorcache.AddOrReplace(objName, mutexForObject, time.Second*10)
		}
		m.Unlock()
		mutexForObject.Lock()
		defer mutexForObject.Unlock()
		req := clientSet.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(podName).
			Namespace(namespace).
			SubResource("exec").VersionedParams(&corev1.PodExecOptions{
			Command:   []string{"/bin/sh", "-c", command},
			Container: containerName,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

		if debug {
			fmt.Println("Request URL:", req.URL().String())
		}

		exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
		if err != nil {
			mErr = fmt.Errorf("error while creating Executor: %v", err)
			return
		}
		var stdout, stderr bytes.Buffer
		err = exec.StreamWithContext(myCtx, remotecommand.StreamOptions{
			Stdin:  stdin,
			Stdout: &stdout,
			Stderr: &stderr,
			Tty:    false,
		})
		result = stdout.String()
		if err != nil {
			mErr = fmt.Errorf("error in Stream: %v\n\nstderr:\n%v\n\nstdout:\n%v", err.Error(), stderr.String(), result)
			return
		}
	}()
	<-myCtx.Done()
	if myCtx.Err() != nil && !strings.Contains(myCtx.Err().Error(), "context canceled") {
		mErr = multierr.Append(mErr, myCtx.Err())
	}
	return result, mErr
}

// RunCommandInPodWithTimeout runs a command in a container with specified timeout.
// Timeout can't be less 1ms
func RunCommandInPodWithTimeout(timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, error) {
	ctx := context.Background()
	return RunCommandInPodWithContextAndTimeout(ctx, timeout, command, containerName, podName, namespace, stdin)
}

// RunCommandInPod runs a command in a container with default 10 sec timeout
func RunCommandInPod(command, containerName, podName, namespace string, stdin io.Reader) (string, error) {
	return RunCommandInPodWithTimeout(time.Second*10, command, containerName, podName, namespace, stdin)
}

func InitializeOperatorCoreClientSet() {
	if clientSet != nil {
		return
	}
	clientSet = kubernetes.NewForConfigOrDie(GetClientConfig())
}

func init() {
	InitializeOperatorCoreClientSet()
}

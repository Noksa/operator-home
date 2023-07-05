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
	var err error
	config, err = ctrl.GetConfig()
	if err != nil {
		panic(fmt.Sprintf("couldn't create kube config: %v", err.Error()))
	}
	return config
}

type RunCommandInPodOptions struct {
	// Background context will be used if not set
	Context context.Context
	// Default value is 10 seconds if not set
	Timeout time.Duration
	// Command to be run
	Command       string
	PodName       string
	PodNamespace  string
	ContainerName string
	Stdin         io.Reader
	Stderr        io.Writer
	Stdout        io.Writer
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

// RunCommandInPodWithOptions returns stdout, stderr, err after running a command
func RunCommandInPodWithOptions(options RunCommandInPodOptions) (string, string, error) {
	if options.Timeout < time.Millisecond*1 {
		options.Timeout = time.Second * 10
	}
	myCtx, cancel := context.WithTimeout(options.Context, options.Timeout)
	var mErr error
	var stdout, stderr string
	go func() {
		defer cancel()
		objName := fmt.Sprintf("%v-%v-%v", options.PodNamespace, options.PodName, options.ContainerName)
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
			Name(options.PodName).
			Namespace(options.PodNamespace).
			SubResource("exec").VersionedParams(&corev1.PodExecOptions{
			Command:   []string{"/bin/sh", "-c", options.Command},
			Container: options.ContainerName,
			Stdin:     options.Stdin != nil,
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
		stdoutBuffer := &bytes.Buffer{}
		stderrBuffer := &bytes.Buffer{}
		var stdoutMultiWriter, stderrMultiWriter io.Writer
		if options.Stdout != nil {
			stdoutMultiWriter = io.MultiWriter(stdoutBuffer, options.Stdout)
		} else {
			stdoutMultiWriter = stdoutBuffer
		}
		if options.Stderr != nil {
			stderrMultiWriter = io.MultiWriter(stderrBuffer, options.Stderr)
		} else {
			stderrMultiWriter = stderrBuffer
		}

		err = exec.StreamWithContext(myCtx, remotecommand.StreamOptions{
			Stdin:  options.Stdin,
			Stdout: stdoutMultiWriter,
			Stderr: stderrMultiWriter,
			Tty:    false,
		})
		stdout = stdoutBuffer.String()
		stderr = stderrBuffer.String()
		stdoutBuffer = nil
		stderrBuffer = nil
		if err != nil {
			mErr = fmt.Errorf("'%v' command failed: %v", options.Command, err.Error())
			return
		}
	}()
	<-myCtx.Done()
	if myCtx.Err() != nil && !strings.Contains(myCtx.Err().Error(), "context canceled") {
		mErr = multierr.Append(mErr, fmt.Errorf("context canceled"))
		mErr = multierr.Append(mErr, myCtx.Err())
	}
	return stdout, stderr, mErr
}

func RunCommandInPodWithContextAndTimeout(ctx context.Context, timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
	return RunCommandInPodWithOptions(RunCommandInPodOptions{
		Context:       ctx,
		Timeout:       timeout,
		Command:       command,
		PodName:       podName,
		PodNamespace:  namespace,
		ContainerName: containerName,
		Stdin:         stdin,
		Stderr:        nil,
		Stdout:        nil,
	})
}

// RunCommandInPodWithTimeout runs a command in a container with specified timeout.
// Timeout can't be less 1ms
func RunCommandInPodWithTimeout(timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
	ctx := context.Background()
	return RunCommandInPodWithContextAndTimeout(ctx, timeout, command, containerName, podName, namespace, stdin)
}

// RunCommandInPod runs a command in a container with default 10 sec timeout
func RunCommandInPod(command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
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

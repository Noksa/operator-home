package operatorkclient

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Noksa/operator-home/internal/operatorcache"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
)

const debug = false

var clientSet kubernetes.Interface
var config *rest.Config
var m sync.Mutex
var initOnce sync.Once
var overridden bool

// SetClientSet allows injecting a custom kubernetes.Interface (e.g. fake.NewClientset())
// for testing. When set, lazy initialization is skipped entirely.
func SetClientSet(client kubernetes.Interface) {
	m.Lock()
	defer m.Unlock()
	clientSet = client
	overridden = true
}

// getClientSet returns the kubernetes client, initializing it lazily on first call.
// If SetClientSet was called, the injected client is returned without initialization.
func getClientSet() kubernetes.Interface {
	m.Lock()
	if overridden {
		cs := clientSet
		m.Unlock()
		return cs
	}
	m.Unlock()
	initOnce.Do(func() {
		clientSet = kubernetes.NewForConfigOrDie(GetClientConfig())
	})
	return clientSet
}

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
	podLogsRequest := getClientSet().CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		SinceTime: sinceTime,
	})
	logStream, err := podLogsRequest.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = logStream.Close() }()
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
				builder.WriteString(line)
				builder.WriteByte('\n')
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
	type execResult struct {
		stdout string
		stderr string
		err    error
	}
	resultChan := make(chan execResult, 1)
	go func() {
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
		req := getClientSet().CoreV1().RESTClient().Post().
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
			resultChan <- execResult{err: fmt.Errorf("error while creating Executor: %v", err)}
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
		so := stdoutBuffer.String()
		se := stderrBuffer.String()
		stdoutBuffer = nil
		stderrBuffer = nil
		if err != nil {
			resultChan <- execResult{stdout: so, stderr: se, err: fmt.Errorf("'%v' command failed: %v", options.Command, err.Error())}
			return
		}
		resultChan <- execResult{stdout: so, stderr: se}
	}()

	var mErr error
	var stdout, stderr string
	select {
	case <-myCtx.Done():
		mErr = multierr.Append(mErr, myCtx.Err())
	case res := <-resultChan:
		stdout = res.stdout
		stderr = res.stderr
		mErr = multierr.Append(mErr, res.err)
	}
	cancel()
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

// InitializeOperatorCoreClientSet eagerly initializes the client set.
// This is kept for backward compatibility but is no longer required —
// the client is initialized lazily on first use.
func InitializeOperatorCoreClientSet() {
	getClientSet()
}

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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
)

const debug = false

var (
	defaultOnce   sync.Once
	defaultCfg    *rest.Config
	defaultInst   *Client
	defaultSealed bool
	defaultMu     sync.Mutex
)

// SetDefaultConfig sets the rest.Config that DefaultClient will use.
// Must be called before the first call to DefaultClient; panics otherwise.
// If never called, DefaultClient falls back to ctrl.GetConfig().
func SetDefaultConfig(cfg *rest.Config) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultSealed {
		panic("operatorkclient: SetDefaultConfig called after DefaultClient was already initialized")
	}
	defaultCfg = cfg
}

// DefaultClient returns a lazily-initialized singleton Client.
// On first call it uses the config from SetDefaultConfig if one was
// provided, otherwise it falls back to the in-cluster / kubeconfig config
// from controller-runtime. Panics if the client cannot be created.
func DefaultClient() *Client {
	defaultOnce.Do(func() {
		defaultMu.Lock()
		defaultSealed = true
		cfg := defaultCfg
		defaultMu.Unlock()

		var err error
		if cfg != nil {
			defaultInst, err = NewClientFromConfig(cfg)
		} else {
			defaultInst, err = NewClient()
		}
		if err != nil {
			panic(fmt.Sprintf("operatorkclient: failed to create default client: %v", err))
		}
	})
	return defaultInst
}

// Client wraps a typed Kubernetes clientset, a dynamic client, cached
// discovery, and a REST mapper behind a single handle.
type Client struct {
	clientSet       kubernetes.Interface
	dynamic         dynamic.Interface
	cachedDiscovery discovery.CachedDiscoveryInterface
	restMapper      *restmapper.DeferredDiscoveryRESTMapper
	config          *rest.Config
	mu              sync.Mutex
}

// NewClient creates a Client using the in-cluster or kubeconfig-based
// rest.Config obtained from controller-runtime.
func NewClient() (*Client, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("couldn't create kube config: %w", err)
	}
	return NewClientFromConfig(cfg)
}

// NewClientFromConfig creates a fully-wired Client from an existing rest.Config.
func NewClientFromConfig(cfg *rest.Config) (*Client, error) {
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't create kubernetes clientset: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("couldn't create dynamic client: %w", err)
	}
	return newClientWithDeps(cs, dyn, cfg), nil
}

// NewClientFromClientSet creates a Client from pre-built typed and dynamic
// clients. Useful for testing with fake.NewClientset() / fakedynamic.
// A non-nil config is required for operations that use SPDY (exec, cp).
func NewClientFromClientSet(cs kubernetes.Interface, dyn dynamic.Interface, cfg *rest.Config) *Client {
	return newClientWithDeps(cs, dyn, cfg)
}

func newClientWithDeps(cs kubernetes.Interface, dyn dynamic.Interface, cfg *rest.Config) *Client {
	cachedDisc := memory.NewMemCacheClient(cs.Discovery())
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDisc)
	return &Client{
		clientSet:       cs,
		dynamic:         dyn,
		cachedDiscovery: cachedDisc,
		restMapper:      mapper,
		config:          cfg,
	}
}

// ClientSet returns the typed kubernetes.Interface.
func (c *Client) ClientSet() kubernetes.Interface {
	return c.clientSet
}

// Dynamic returns the dynamic Kubernetes client.
func (c *Client) Dynamic() dynamic.Interface {
	return c.dynamic
}

// Discovery returns the cached discovery client.
func (c *Client) Discovery() discovery.CachedDiscoveryInterface {
	return c.cachedDiscovery
}

// RESTMapper returns the deferred discovery REST mapper.
func (c *Client) RESTMapper() *restmapper.DeferredDiscoveryRESTMapper {
	return c.restMapper
}

// Config returns the underlying rest.Config.
func (c *Client) Config() *rest.Config {
	return c.config
}

// RunCommandInPodOptions holds parameters for running a command inside a pod container.
type RunCommandInPodOptions struct {
	Context       context.Context
	Timeout       time.Duration // defaults to 10s if zero
	Command       string
	PodName       string
	PodNamespace  string
	ContainerName string
	Stdin         io.Reader
	Stderr        io.Writer
	Stdout        io.Writer
}

// GetPodContainerLogs retrieves logs from a specific container in a pod.
func (c *Client) GetPodContainerLogs(ctx context.Context, namespace, podName, containerName string, sinceTime *metav1.Time) (string, error) {
	podLogsRequest := c.clientSet.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		SinceTime: sinceTime,
	})
	logStream, err := podLogsRequest.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = logStream.Close() }()

	var builder strings.Builder
	reader := bufio.NewScanner(logStream)
	t := time.Now()
	for time.Since(t) <= time.Minute {
		select {
		case <-ctx.Done():
			return builder.String(), nil
		default:
			for reader.Scan() {
				builder.WriteString(reader.Text())
				builder.WriteByte('\n')
			}
			return builder.String(), nil
		}
	}
	return "", fmt.Errorf("timed out in GetPodContainerLogs")
}

// RunCommandInPodWithOptions executes a command in a pod container and returns stdout, stderr, and any error.
func (c *Client) RunCommandInPodWithOptions(options RunCommandInPodOptions) (string, string, error) {
	if options.Timeout < time.Millisecond {
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
		c.mu.Lock()
		mutexForObject, found := operatorcache.Get[*sync.Mutex](objName)
		if !found {
			mutexForObject = &sync.Mutex{}
			operatorcache.AddOrReplace(objName, mutexForObject, time.Second*10)
		}
		c.mu.Unlock()
		mutexForObject.Lock()
		defer mutexForObject.Unlock()

		req := c.clientSet.CoreV1().RESTClient().Post().
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

		exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
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

// RunCommandInPodWithContextAndTimeout is a convenience wrapper.
func (c *Client) RunCommandInPodWithContextAndTimeout(ctx context.Context, timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
	return c.RunCommandInPodWithOptions(RunCommandInPodOptions{
		Context:       ctx,
		Timeout:       timeout,
		Command:       command,
		PodName:       podName,
		PodNamespace:  namespace,
		ContainerName: containerName,
		Stdin:         stdin,
	})
}

// RunCommandInPodWithTimeout runs a command in a container with the specified timeout.
func (c *Client) RunCommandInPodWithTimeout(timeout time.Duration, command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
	return c.RunCommandInPodWithContextAndTimeout(context.Background(), timeout, command, containerName, podName, namespace, stdin)
}

// RunCommandInPod runs a command in a container with a default 10s timeout.
func (c *Client) RunCommandInPod(command, containerName, podName, namespace string, stdin io.Reader) (string, string, error) {
	return c.RunCommandInPodWithTimeout(time.Second*10, command, containerName, podName, namespace, stdin)
}

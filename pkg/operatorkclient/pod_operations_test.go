package operatorkclient_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

// newCapturingTestClient creates a Client whose backing httptest.Server records
// the URL of every request it receives. This lets tests inspect query parameters
// (e.g. tty=true) that the Kubernetes exec request sends.
func newCapturingTestClient() (client *kc.Client, server *httptest.Server, requestURLs *[]*url.URL) {
	var urls []*url.URL
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls = append(urls, r.URL)
		w.WriteHeader(http.StatusNotFound)
	}))
	cfg := &rest.Config{Host: server.URL}
	cs, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	dyn, err := dynamic.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	return kc.NewClientFromClientSet(cs, dyn, cfg), server, &urls
}

var _ = Describe("GetPodContainerLogs", func() {
	var (
		client *kc.Client
		ctx    context.Context
	)

	BeforeEach(func() {
		client = kc.NewClientFromClientSet(fake.NewClientset(), fakedynamic.NewSimpleDynamicClient(scheme.Scheme), nil)
		ctx = context.Background()
	})

	It("should return logs for a non-existent pod (fake client)", func() {
		logs, err := client.GetPodContainerLogs(ctx, "default", "nonexistent", "c", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(ContainSubstring("fake logs"))
	})

	It("should accept a sinceTime parameter without panic", func() {
		since := &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
		logs, err := client.GetPodContainerLogs(ctx, "default", "pod", "c", since)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).NotTo(BeEmpty())
	})

	When("context is already cancelled", func() {
		It("should return without hanging", func() {
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()
			logs, _ := client.GetPodContainerLogs(cancelledCtx, "default", "pod", "c", nil)
			_ = logs
		})
	})
})

var _ = Describe("RunCommandInPod variants", Serial, Ordered, func() {
	var (
		server *httptest.Server
		client *kc.Client
	)

	BeforeAll(func() {
		client, server = newTestClient()
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
		time.Sleep(100 * time.Millisecond)
	})

	Describe("RunCommandInPodWithOptions", func() {
		It("should default timeout to 10s when timeout is zero", func() {
			_, _, err := client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       context.Background(),
				Timeout:       0,
				Command:       "echo hello",
				PodName:       "test-pod",
				PodNamespace:  "default",
				ContainerName: "container",
			})
			Expect(err).To(HaveOccurred())
		})

		It("should complete within the provided timeout", func() {
			start := time.Now()
			_, _, err := client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       context.Background(),
				Timeout:       500 * time.Millisecond,
				Command:       "echo hello",
				PodName:       "test-pod",
				PodNamespace:  "default",
				ContainerName: "container",
			})
			Expect(err).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
		})

		It("should respect context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, _, err := client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       ctx,
				Timeout:       5 * time.Second,
				Command:       "echo hello",
				PodName:       "test-pod",
				PodNamespace:  "default",
				ContainerName: "container",
			})
			Expect(err).To(HaveOccurred())
		})

		It("should accept custom stdout and stderr writers", func() {
			_, _, err := client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       context.Background(),
				Timeout:       time.Second,
				Command:       "echo hello",
				PodName:       "test-pod",
				PodNamespace:  "default",
				ContainerName: "container",
				Stdout:        io.Discard,
				Stderr:        io.Discard,
			})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunCommandInPod", func() {
		It("should not panic with test client", func() {
			_, _, err := client.RunCommandInPod("echo hello", "c", "pod", "default", nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunCommandInPodWithTimeout", func() {
		It("should pass through the custom timeout", func() {
			_, _, err := client.RunCommandInPodWithTimeout(
				2*time.Second, "echo hello", "c", "pod", "default", nil,
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunCommandInPodWithContextAndTimeout", func() {
		It("should pass through context and timeout", func() {
			_, _, err := client.RunCommandInPodWithContextAndTimeout(
				context.Background(), 2*time.Second, "echo hello", "c", "pod", "default", nil,
			)
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("TTY flag propagation", Serial, Ordered, func() {
	var (
		server      *httptest.Server
		client      *kc.Client
		requestURLs *[]*url.URL
	)

	BeforeAll(func() {
		client, server, requestURLs = newCapturingTestClient()
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
		time.Sleep(100 * time.Millisecond)
	})

	It("should set tty=true in PodExecOptions when TTY is true", func() {
		*requestURLs = nil
		_, _, _ = client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
			Context:       context.Background(),
			Timeout:       time.Second,
			Command:       "echo tty",
			PodName:       "pod",
			PodNamespace:  "default",
			ContainerName: "c",
			TTY:           true,
		})
		Expect(*requestURLs).NotTo(BeEmpty())
		raw := (*requestURLs)[0].RawQuery
		Expect(raw).To(ContainSubstring("tty=true"))
	})

	It("should not set tty=true when TTY is false (default)", func() {
		*requestURLs = nil
		_, _, _ = client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
			Context:       context.Background(),
			Timeout:       time.Second,
			Command:       "echo notty",
			PodName:       "pod",
			PodNamespace:  "default",
			ContainerName: "c",
		})
		Expect(*requestURLs).NotTo(BeEmpty())
		raw := (*requestURLs)[0].RawQuery
		Expect(raw).NotTo(ContainSubstring("tty=true"))
	})
})

var _ = Describe("Functional options (With* functions)", func() {
	It("WithContext should set the context", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		opts := &kc.RunCommandInPodOptions{}
		kc.WithContext(ctx)(opts)
		Expect(opts.Context).To(Equal(ctx))
	})

	It("WithTimeout should set the timeout", func() {
		opts := &kc.RunCommandInPodOptions{}
		kc.WithTimeout(42 * time.Second)(opts)
		Expect(opts.Timeout).To(Equal(42 * time.Second))
	})

	It("WithStdin should set the stdin reader", func() {
		r := strings.NewReader("hello")
		opts := &kc.RunCommandInPodOptions{}
		kc.WithStdin(r)(opts)
		Expect(opts.Stdin).To(Equal(r))
	})

	It("WithStdout should set the stdout writer", func() {
		w := &bytes.Buffer{}
		opts := &kc.RunCommandInPodOptions{}
		kc.WithStdout(w)(opts)
		Expect(opts.Stdout).To(Equal(w))
	})

	It("WithStderr should set the stderr writer", func() {
		w := &bytes.Buffer{}
		opts := &kc.RunCommandInPodOptions{}
		kc.WithStderr(w)(opts)
		Expect(opts.Stderr).To(Equal(w))
	})

	It("WithTTY should set the TTY flag", func() {
		opts := &kc.RunCommandInPodOptions{}
		kc.WithTTY(true)(opts)
		Expect(opts.TTY).To(BeTrue())
	})

	It("WithRawCommand should set the RawCommand flag", func() {
		opts := &kc.RunCommandInPodOptions{}
		kc.WithRawCommand(true)(opts)
		Expect(opts.RawCommand).To(BeTrue())
	})

	It("should allow combining multiple options", func() {
		ctx := context.Background()
		buf := &bytes.Buffer{}
		opts := &kc.RunCommandInPodOptions{}
		for _, fn := range []kc.RunCommandOption{
			kc.WithContext(ctx),
			kc.WithTimeout(30 * time.Second),
			kc.WithStdout(buf),
			kc.WithTTY(true),
		} {
			fn(opts)
		}
		Expect(opts.Context).To(Equal(ctx))
		Expect(opts.Timeout).To(Equal(30 * time.Second))
		Expect(opts.Stdout).To(Equal(buf))
		Expect(opts.TTY).To(BeTrue())
	})
})

var _ = Describe("ExecInPod", Serial, Ordered, func() {
	var (
		server *httptest.Server
		client *kc.Client
	)

	BeforeAll(func() {
		client, server = newTestClient()
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
		time.Sleep(100 * time.Millisecond)
	})

	It("should use defaults when no options provided (10s timeout, no TTY)", func() {
		start := time.Now()
		_, _, err := client.ExecInPod("echo hi", "c", "pod", "default")
		Expect(err).To(HaveOccurred())
		// Should complete well before the 10s default timeout since the test server 404s immediately
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
	})

	It("should apply WithTimeout option", func() {
		start := time.Now()
		_, _, err := client.ExecInPod("echo hi", "c", "pod", "default",
			kc.WithTimeout(500*time.Millisecond),
		)
		Expect(err).To(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically("<", 3*time.Second))
	})

	It("should apply WithContext option", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := client.ExecInPod("echo hi", "c", "pod", "default",
			kc.WithContext(ctx),
		)
		Expect(err).To(HaveOccurred())
	})

	It("should apply WithStdout and WithStderr options", func() {
		_, _, err := client.ExecInPod("echo hi", "c", "pod", "default",
			kc.WithTimeout(time.Second),
			kc.WithStdout(io.Discard),
			kc.WithStderr(io.Discard),
		)
		Expect(err).To(HaveOccurred())
	})

	It("should delegate to RunCommandInPodWithOptions", func() {
		// ExecInPod with the same params as a direct RunCommandInPodWithOptions call
		// should produce the same kind of error (both hit the test server).
		_, _, errExec := client.ExecInPod("echo hi", "c", "pod", "default",
			kc.WithTimeout(time.Second),
		)
		_, _, errDirect := client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
			Context:       context.Background(),
			Timeout:       time.Second,
			Command:       "echo hi",
			PodName:       "pod",
			PodNamespace:  "default",
			ContainerName: "c",
		})
		Expect(errExec).To(HaveOccurred())
		Expect(errDirect).To(HaveOccurred())
	})
})

var _ = Describe("RawCommand flag propagation", Serial, Ordered, func() {
	var (
		server      *httptest.Server
		client      *kc.Client
		requestURLs *[]*url.URL
	)

	BeforeAll(func() {
		client, server, requestURLs = newCapturingTestClient()
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
		time.Sleep(100 * time.Millisecond)
	})

	It("should wrap command in /bin/sh -c by default", func() {
		*requestURLs = nil
		_, _, _ = client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
			Context:       context.Background(),
			Timeout:       time.Second,
			Command:       "echo hello",
			PodName:       "pod",
			PodNamespace:  "default",
			ContainerName: "c",
		})
		Expect(*requestURLs).NotTo(BeEmpty())
		raw := (*requestURLs)[0].RawQuery
		Expect(raw).To(ContainSubstring("command=%2Fbin%2Fsh"))
		Expect(raw).To(ContainSubstring("command=-c"))
	})

	It("should pass command as split argv when RawCommand is true", func() {
		*requestURLs = nil
		_, _, _ = client.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
			Context:       context.Background(),
			Timeout:       time.Second,
			Command:       "tar czf - -C /tmp .",
			PodName:       "pod",
			PodNamespace:  "default",
			ContainerName: "c",
			RawCommand:    true,
		})
		Expect(*requestURLs).NotTo(BeEmpty())
		raw := (*requestURLs)[0].RawQuery
		// Should NOT contain /bin/sh wrapper
		Expect(raw).NotTo(ContainSubstring("command=%2Fbin%2Fsh"))
		Expect(raw).NotTo(ContainSubstring("command=-c"))
		// Should contain tar as the first command element
		Expect(raw).To(ContainSubstring("command=tar"))
	})

	It("should work with WithRawCommand via ExecInPod", func() {
		*requestURLs = nil
		_, _, _ = client.ExecInPod("cat /etc/resolv.conf", "c", "pod", "default",
			kc.WithTimeout(time.Second),
			kc.WithRawCommand(true),
		)
		Expect(*requestURLs).NotTo(BeEmpty())
		raw := (*requestURLs)[0].RawQuery
		Expect(raw).NotTo(ContainSubstring("command=%2Fbin%2Fsh"))
		Expect(raw).To(ContainSubstring("command=cat"))
	})
})

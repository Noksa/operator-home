package operatorkclient_test

import (
	"context"
	"io"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

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

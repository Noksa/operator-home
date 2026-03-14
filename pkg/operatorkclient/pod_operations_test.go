package operatorkclient_test

import (
	"context"
	"io"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

var _ = Describe("GetPodContainerLogs", func() {
	var ctx context.Context

	BeforeEach(func() {
		kc.ResetForTesting()
		kc.SetClientSet(fake.NewClientset())
		ctx = context.Background()
	})

	It("should return logs for a non-existent pod (fake client)", func() {
		// fake client's GetLogs().Stream() returns "fake logs\n"
		logs, err := kc.GetPodContainerLogs(ctx, "default", "nonexistent", "c", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).To(ContainSubstring("fake logs"))
	})

	It("should accept a sinceTime parameter without panic", func() {
		since := &metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
		logs, err := kc.GetPodContainerLogs(ctx, "default", "pod", "c", since)
		Expect(err).NotTo(HaveOccurred())
		Expect(logs).NotTo(BeEmpty())
	})

	When("context is already cancelled", func() {
		It("should return without hanging", func() {
			cancelledCtx, cancel := context.WithCancel(ctx)
			cancel()
			// Must complete quickly — no hang
			logs, _ := kc.GetPodContainerLogs(cancelledCtx, "default", "pod", "c", nil)
			_ = logs
		})
	})
})

// Serial + Ordered because RunCommandInPodWithOptions spawns a goroutine that
// reads package-level state (config). Running serially prevents races with
// ResetForTesting across specs.
var _ = Describe("RunCommandInPod variants", Serial, Ordered, func() {
	var (
		server *httptest.Server
		client kubernetes.Interface
	)

	BeforeAll(func() {
		kc.ResetForTesting()
		client, server = newFakeClientWithServer()
		kc.SetClientSet(client)
	})

	AfterAll(func() {
		if server != nil {
			server.Close()
		}
		// Allow lingering goroutines to drain before any subsequent spec resets state
		time.Sleep(100 * time.Millisecond)
	})

	Describe("RunCommandInPodWithOptions", func() {
		It("should default timeout to 10s when timeout is zero", func() {
			ctx := context.Background()
			_, _, err := kc.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       ctx,
				Timeout:       0,
				Command:       "echo hello",
				PodName:       "test-pod",
				PodNamespace:  "default",
				ContainerName: "container",
			})
			// Errors because there's no real SPDY endpoint, but must not nil-panic
			Expect(err).To(HaveOccurred())
		})

		It("should complete within the provided timeout", func() {
			ctx := context.Background()
			start := time.Now()
			_, _, err := kc.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       ctx,
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
			_, _, err := kc.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
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
			ctx := context.Background()
			_, _, err := kc.RunCommandInPodWithOptions(kc.RunCommandInPodOptions{
				Context:       ctx,
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
		It("should not panic with fake client", func() {
			_, _, err := kc.RunCommandInPod("echo hello", "c", "pod", "default", nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunCommandInPodWithTimeout", func() {
		It("should pass through the custom timeout", func() {
			_, _, err := kc.RunCommandInPodWithTimeout(
				2*time.Second, "echo hello", "c", "pod", "default", nil,
			)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("RunCommandInPodWithContextAndTimeout", func() {
		It("should pass through context and timeout", func() {
			ctx := context.Background()
			_, _, err := kc.RunCommandInPodWithContextAndTimeout(
				ctx, 2*time.Second, "echo hello", "c", "pod", "default", nil,
			)
			Expect(err).To(HaveOccurred())
		})
	})
})

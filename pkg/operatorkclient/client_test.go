package operatorkclient_test

import (
	"os"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/fake"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

var _ = Describe("Lazy Initialization", func() {
	BeforeEach(func() {
		kc.ResetForTesting()
	})

	Context("when SetClientSet is called", func() {
		It("should skip real kubeconfig initialization", func() {
			kc.SetClientSet(fake.NewClientset())
			Expect(func() { kc.InitializeOperatorCoreClientSet() }).NotTo(Panic())
		})

		It("should allow replacing the client", func() {
			kc.SetClientSet(fake.NewClientset())
			kc.SetClientSet(fake.NewClientset())
			Expect(func() { kc.InitializeOperatorCoreClientSet() }).NotTo(Panic())
		})

		It("should be safe to call concurrently", func() {
			var wg sync.WaitGroup
			for range 50 {
				wg.Add(1)
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					kc.SetClientSet(fake.NewClientset())
				}()
			}
			wg.Wait()
		})
	})

	Context("when InitializeOperatorCoreClientSet is called", func() {
		It("should not panic with an injected client", func() {
			kc.SetClientSet(fake.NewClientset())
			Expect(func() { kc.InitializeOperatorCoreClientSet() }).NotTo(Panic())
		})

		It("should be idempotent", func() {
			kc.SetClientSet(fake.NewClientset())
			Expect(func() {
				kc.InitializeOperatorCoreClientSet()
				kc.InitializeOperatorCoreClientSet()
				kc.InitializeOperatorCoreClientSet()
			}).NotTo(Panic())
		})
	})

	Context("import safety", func() {
		It("should not panic at import time without kubeconfig", func() {
			// This test compiling and running proves the init() panic is gone.
			Succeed()
		})
	})
})

var _ = Describe("GetClientConfig", func() {
	BeforeEach(func() {
		kc.ResetForTesting()
	})

	When("KUBECONFIG points to a nonexistent file", func() {
		var origKubeconfig string
		var hadKubeconfig bool

		BeforeEach(func() {
			origKubeconfig, hadKubeconfig = os.LookupEnv("KUBECONFIG")
			Expect(os.Setenv("KUBECONFIG", "/nonexistent/path/kubeconfig")).To(Succeed())
		})

		AfterEach(func() {
			if hadKubeconfig {
				Expect(os.Setenv("KUBECONFIG", origKubeconfig)).To(Succeed())
			} else {
				Expect(os.Unsetenv("KUBECONFIG")).To(Succeed())
			}
		})

		It("should panic", func() {
			Expect(func() { kc.GetClientConfig() }).To(Panic())
		})
	})
})

package operatorkclient_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

var _ = Describe("Client construction", func() {
	Context("NewClientFromConfig", func() {
		It("should return an error for an invalid config", func() {
			_, err := kc.NewClientFromConfig(&rest.Config{Host: "http://[::1]:namedport"})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("NewClientFromClientSet", func() {
		It("should expose the injected clientset, dynamic client, and config", func() {
			cs := fake.NewClientset()
			dyn := fakedynamic.NewSimpleDynamicClient(scheme.Scheme)
			cfg := &rest.Config{Host: "https://localhost:6443"}
			c := kc.NewClientFromClientSet(cs, dyn, cfg)
			Expect(c.ClientSet()).To(Equal(cs))
			Expect(c.Dynamic()).To(Equal(dyn))
			Expect(c.Config()).To(Equal(cfg))
			Expect(c.Discovery()).NotTo(BeNil())
			Expect(c.RESTMapper()).NotTo(BeNil())
		})
	})
})

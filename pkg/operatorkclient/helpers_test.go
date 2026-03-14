package operatorkclient_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

// newTestClient creates a Client backed by a real httptest.Server so that
// CoreV1().RESTClient() is non-nil and SPDY executor creation doesn't panic.
// Returns the client and the server. Caller must close the server.
func newTestClient() (*kc.Client, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	cfg := &rest.Config{Host: server.URL}
	cs, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	dyn, err := dynamic.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	return kc.NewClientFromClientSet(cs, dyn, cfg), server
}

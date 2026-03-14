package operatorkclient_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	kc "github.com/Noksa/operator-home/pkg/operatorkclient"
)

// newFakeClientWithServer creates a kubernetes.Interface backed by a real
// httptest.Server so that CoreV1().RESTClient() is non-nil.
// It also sets the package-level config via SetConfigForTesting so that
// NewSPDYExecutor doesn't receive a nil *rest.Config.
//
// Returns the client and the server. Caller must close the server.
func newFakeClientWithServer() (kubernetes.Interface, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	cfg := &rest.Config{Host: server.URL}
	client, err := kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	kc.SetConfigForTesting(cfg)
	return client, server
}

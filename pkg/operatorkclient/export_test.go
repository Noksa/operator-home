package operatorkclient

import (
	"sync"

	"k8s.io/client-go/rest"
)

// ResetForTesting resets all package-level state so each test starts clean.
// Only available in _test builds.
func ResetForTesting() {
	m.Lock()
	defer m.Unlock()
	clientSet = nil
	config = nil
	initOnce = sync.Once{}
	overridden = false
}

// SetConfigForTesting sets the package-level rest.Config used by SPDY executors.
// Only available in _test builds.
func SetConfigForTesting(cfg *rest.Config) {
	config = cfg
}

package operatormulticluster

import (
	"context"
	"fmt"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

// ClusterConfig holds the identifying name and kubeconfig path for a remote cluster.
type ClusterConfig struct {
	Name       string
	Kubeconfig string
}

// StaticProvider manages a fixed set of remote clusters from kubeconfig files.
// It implements multicluster.Provider and multicluster.ProviderRunnable, so it
// can drive a multicluster manager or be used standalone via Clusters().
type StaticProvider struct {
	configs     []ClusterConfig
	clusterOpts []cluster.Option
	restMod     func(*rest.Config)

	mu       sync.RWMutex
	clusters map[multicluster.ClusterName]cluster.Cluster
	ready    chan struct{}
}

// NewStaticProvider creates a provider that manages the given cluster configs.
// Pass cluster.Option values to customize each cluster (scheme, REST mapper, etc.).
func NewStaticProvider(configs []ClusterConfig, opts ...cluster.Option) *StaticProvider {
	return &StaticProvider{
		configs:     configs,
		clusterOpts: opts,
		clusters:    make(map[multicluster.ClusterName]cluster.Cluster, len(configs)),
		ready:       make(chan struct{}),
	}
}

// WithRestConfigModifier sets a function applied to each cluster's *rest.Config
// before the cluster is created. Use this to set transport timeouts, dial
// settings, or other properties that cluster.Option cannot express.
func (p *StaticProvider) WithRestConfigModifier(fn func(*rest.Config)) *StaticProvider {
	p.restMod = fn
	return p
}

// Start implements multicluster.ProviderRunnable. For each configured cluster it
// loads the kubeconfig, applies any rest.Config modifier, creates the cluster,
// starts its cache, waits for sync, stores the handle, and calls aware.Engage.
// Blocks until ctx is done. If aware is nil, Engage calls are skipped (standalone
// mode — use Clusters() to retrieve handles after WaitReady returns).
func (p *StaticProvider) Start(ctx context.Context, aware multicluster.Aware) error {
	for _, cc := range p.configs {
		restCfg, err := clientcmd.BuildConfigFromFlags("", cc.Kubeconfig)
		if err != nil {
			return fmt.Errorf("cluster %s: load kubeconfig: %w", cc.Name, err)
		}
		if p.restMod != nil {
			p.restMod(restCfg)
		}

		cl, err := cluster.New(restCfg, p.clusterOpts...)
		if err != nil {
			return fmt.Errorf("cluster %s: create: %w", cc.Name, err)
		}

		go func(name string) {
			if err := cl.Start(ctx); err != nil && ctx.Err() == nil {
				// Log but don't panic — the cache WaitForCacheSync below will surface it.
				_ = err
			}
		}(cc.Name)

		if !cl.GetCache().WaitForCacheSync(ctx) {
			return fmt.Errorf("cluster %s: cache sync timed out", cc.Name)
		}

		name := multicluster.ClusterName(cc.Name)
		p.mu.Lock()
		p.clusters[name] = cl
		p.mu.Unlock()

		if aware != nil {
			if err := aware.Engage(ctx, name, cl); err != nil {
				return fmt.Errorf("cluster %s: engage: %w", cc.Name, err)
			}
		}
	}

	close(p.ready)
	<-ctx.Done()
	return nil
}

// WaitReady blocks until all clusters have been started and synced, or ctx is
// cancelled. Call this after launching Start in a goroutine to gate on readiness.
func (p *StaticProvider) WaitReady(ctx context.Context) error {
	select {
	case <-p.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Clusters returns a snapshot of all currently engaged clusters keyed by name.
// Only valid after WaitReady returns nil.
func (p *StaticProvider) Clusters() map[string]cluster.Cluster {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]cluster.Cluster, len(p.clusters))
	for k, v := range p.clusters {
		out[string(k)] = v
	}
	return out
}

// Get implements multicluster.Provider.
func (p *StaticProvider) Get(_ context.Context, name multicluster.ClusterName) (cluster.Cluster, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cl, ok := p.clusters[name]
	if !ok {
		return nil, multicluster.ErrClusterNotFound
	}
	return cl, nil
}

// IndexField implements multicluster.Provider. It indexes the given field on
// every currently engaged cluster.
func (p *StaticProvider) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, cl := range p.clusters {
		if err := cl.GetFieldIndexer().IndexField(ctx, obj, field, extractValue); err != nil {
			return err
		}
	}
	return nil
}

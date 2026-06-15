package operatormulticluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

const (
	// SecretFinalizerName is added to each managed Secret to allow cleanup before deletion.
	SecretFinalizerName = "operatormulticluster.ooma.com/cluster-engaged"

	// SecretClusterNameLabel identifies the remote cluster name on the Secret.
	SecretClusterNameLabel = "cluster-mesh-operator.ooma.com/cluster-name"
)

// ClusterRemovedFunc is called before a cluster is disengaged, allowing cleanup.
type ClusterRemovedFunc func(ctx context.Context, clusterName string, cl cluster.Cluster) error

// SecretFinalizer is a controller that manages finalizers on kubeconfig Secrets.
// It works alongside the built-in kubeconfig.Provider — the provider handles
// cluster lifecycle; this controller handles cleanup before Secret deletion.
type SecretFinalizer struct {
	client    client.Client
	namespace string
	labelKey  string
	onRemoved ClusterRemovedFunc
	provider  multicluster.Provider
	log       logr.Logger
}

// NewSecretFinalizer creates a finalizer controller for kubeconfig Secrets.
// It adds a finalizer on create, and calls onRemoved + removes the finalizer on delete.
func NewSecretFinalizer(cl client.Client, namespace, labelKey string, provider multicluster.Provider, onRemoved ClusterRemovedFunc) *SecretFinalizer {
	return &SecretFinalizer{
		client:    cl,
		namespace: namespace,
		labelKey:  labelKey,
		onRemoved: onRemoved,
		provider:  provider,
	}
}

// SetupWithManager registers the finalizer controller.
func (f *SecretFinalizer) SetupWithManager(mgr manager.Manager) error {
	f.log = mgr.GetLogger().WithName("secret-finalizer")
	return builder.ControllerManagedBy(mgr).
		For(&corev1.Secret{}, builder.WithPredicates(predicate.NewPredicateFuncs(
			func(obj client.Object) bool {
				return obj.GetNamespace() == f.namespace &&
					obj.GetLabels()[f.labelKey] == "true"
			},
		))).
		Named("secret-finalizer").
		Complete(f)
}

func (f *SecretFinalizer) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	secret := &corev1.Secret{}
	if err := f.client.Get(ctx, req.NamespacedName, secret); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	clusterName := clusterNameFromSecret(secret)

	// Being deleted — run cleanup, remove finalizer
	if !secret.DeletionTimestamp.IsZero() {
		if containsFinalizer(secret, SecretFinalizerName) {
			if err := f.cleanup(ctx, clusterName, logger); err != nil {
				return reconcile.Result{}, err
			}
			secret.Finalizers = removeFinalizer(secret.Finalizers, SecretFinalizerName)
			if err := f.client.Update(ctx, secret); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Ensure finalizer is present
	if !containsFinalizer(secret, SecretFinalizerName) {
		secret.Finalizers = append(secret.Finalizers, SecretFinalizerName)
		if err := f.client.Update(ctx, secret); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (f *SecretFinalizer) cleanup(ctx context.Context, clusterName string, logger logr.Logger) error {
	if f.onRemoved == nil {
		return nil
	}

	cl, err := f.provider.Get(ctx, multicluster.ClusterName(clusterName))
	if err != nil {
		// Cluster might already be gone — still run cleanup with nil cluster
		logger.Info("Cluster already removed, running cleanup without cluster handle", "cluster", clusterName)
		return f.onRemoved(ctx, clusterName, nil)
	}

	logger.Info("Running cleanup for cluster", "cluster", clusterName)
	return f.onRemoved(ctx, clusterName, cl)
}

func clusterNameFromSecret(secret *corev1.Secret) string {
	// Must match kubeconfig.Provider which uses secret.Name as ClusterName
	return secret.Name
}

func containsFinalizer(obj client.Object, finalizer string) bool {
	for _, f := range obj.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, finalizer string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f != finalizer {
			result = append(result, f)
		}
	}
	return result
}

// ClusterNameFromSecret is exported for use by consumers to derive the cluster name.
func ClusterNameFromSecret(secret *corev1.Secret) string {
	return clusterNameFromSecret(secret)
}

// UseLabel returns a label value for use in kubeconfig provider options.
// Exported for documentation only — consumers should use the constant directly.
func UseLabel() string {
	return fmt.Sprintf("%s=true", SecretClusterNameLabel)
}

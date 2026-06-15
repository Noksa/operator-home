package operatormulticluster

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/multicluster-runtime/pkg/multicluster"
)

func TestSecretFinalizer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SecretFinalizer Suite")
}

type fakeProvider struct{}

func (f *fakeProvider) Get(_ context.Context, _ multicluster.ClusterName) (cluster.Cluster, error) {
	return nil, multicluster.ErrClusterNotFound
}
func (f *fakeProvider) IndexField(_ context.Context, _ client.Object, _ string, _ client.IndexerFunc) error {
	return nil
}

var _ = Describe("SecretFinalizer", func() {
	const (
		ns       = "cluster-mesh"
		labelKey = "cluster-mesh-operator.ooma.com/kubeconfig"
	)

	Describe("Reconcile", func() {
		It("adds finalizer on first reconcile", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-mesh-kubeconfig-test",
					Namespace: ns,
					Labels:    map[string]string{labelKey: "true", SecretClusterNameLabel: "test"},
				},
			}
			cl := fake.NewClientBuilder().WithObjects(secret).Build()
			f := NewSecretFinalizer(cl, ns, labelKey, &fakeProvider{}, nil)
			f.log = logr.Discard()

			_, err := f.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: ns, Name: secret.Name},
			})
			Expect(err).ToNot(HaveOccurred())

			updated := &corev1.Secret{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: secret.Name}, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(SecretFinalizerName))
		})

		It("removes finalizer and calls cleanup on deletion", func() {
			now := metav1.Now()
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "cluster-mesh-kubeconfig-test",
					Namespace:         ns,
					Labels:            map[string]string{labelKey: "true", SecretClusterNameLabel: "test"},
					Finalizers:        []string{SecretFinalizerName},
					DeletionTimestamp: &now,
				},
			}
			cl := fake.NewClientBuilder().WithObjects(secret).Build()

			var cleanedUp string
			onRemoved := func(_ context.Context, name string, _ cluster.Cluster) error {
				cleanedUp = name
				return nil
			}

			f := NewSecretFinalizer(cl, ns, labelKey, &fakeProvider{}, onRemoved)
			f.log = logr.Discard()

			_, err := f.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: ns, Name: secret.Name},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(cleanedUp).To(Equal("cluster-mesh-kubeconfig-test"))
		})

		It("handles not-found gracefully", func() {
			cl := fake.NewClientBuilder().Build()
			f := NewSecretFinalizer(cl, ns, labelKey, &fakeProvider{}, nil)
			f.log = logr.Discard()

			_, err := f.Reconcile(context.Background(), reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: ns, Name: "gone"},
			})
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("clusterNameFromSecret", func() {
		It("always uses secret name (matches kubeconfig.Provider)", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "cluster-mesh-kubeconfig-milan",
					Labels: map[string]string{SecretClusterNameLabel: "milan"},
				},
			}
			Expect(clusterNameFromSecret(secret)).To(Equal("cluster-mesh-kubeconfig-milan"))
		})
	})

	Describe("containsFinalizer", func() {
		It("returns true when present", func() {
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"a", SecretFinalizerName}}}
			Expect(containsFinalizer(obj, SecretFinalizerName)).To(BeTrue())
		})
		It("returns false when absent", func() {
			obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"a"}}}
			Expect(containsFinalizer(obj, SecretFinalizerName)).To(BeFalse())
		})
	})

	Describe("removeFinalizer", func() {
		It("removes the finalizer", func() {
			Expect(removeFinalizer([]string{"a", SecretFinalizerName, "b"}, SecretFinalizerName)).To(Equal([]string{"a", "b"}))
		})
	})
})

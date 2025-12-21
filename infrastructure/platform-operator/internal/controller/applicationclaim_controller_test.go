package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
)

var _ = Describe("ApplicationClaim Controller", func() {
	const (
		testNamespace = "test-namespace"
		timeout       = time.Second * 10
		interval      = time.Millisecond * 250
	)

	var (
		ctx        context.Context
		reconciler *ApplicationClaimReconciler
		fakeClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Register platform API scheme
		s := scheme.Scheme
		Expect(platformv1.AddToScheme(s)).To(Succeed())

		// Create fake client
		fakeClient = fake.NewClientBuilder().
			WithScheme(s).
			Build()

		// Create reconciler with fake client
		reconciler = &ApplicationClaimReconciler{
			Client: fakeClient,
			Scheme: s,
		}
	})

	Context("When reconciling an ApplicationClaim", func() {
		It("Should create namespace if it doesn't exist", func() {
			// Create ApplicationClaim
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "development",
					Owner: platformv1.OwnerSpec{
						Team:  "Test Team",
						Email: "test@example.com",
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))

			// Check namespace was created
			namespace := &corev1.Namespace{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: testNamespace}, namespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespace.Name).To(Equal(testNamespace))
			Expect(namespace.Labels["platform.infraforge.io/managed"]).To(Equal("true"))
		})

		It("Should deploy backend service when specified", func() {
			// Create ApplicationClaim with backend service
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-backend",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "production",
					Owner: platformv1.OwnerSpec{
						Team:  "Backend Team",
						Email: "backend@example.com",
					},
					Applications: []platformv1.ApplicationSpec{
						{
							Name:    "backend-service",
							Version: "v1.0.0",
							Replicas: func() *int32 {
								r := int32(3)
								return &r
							}(),
							Repository: "github.com/example/backend",
							Ports: []platformv1.PortSpec{
								{
									Name:     "http",
									Port:     8080,
									Protocol: "TCP",
								},
							},
							Env: []platformv1.EnvVar{
								{
									Name:  "ENVIRONMENT",
									Value: "production",
								},
								{
									Name:  "LOG_LEVEL",
									Value: "info",
								},
							},
						},
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Create namespace first
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			}
			Expect(fakeClient.Create(ctx, namespace)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))

			// Check deployment was created
			deployment := &appsv1.Deployment{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "backend-service",
				Namespace: testNamespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("backend-service:v1.0.0"))

			// Check service was created
			service := &corev1.Service{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "backend-service",
				Namespace: testNamespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
		})

		It("Should handle missing ApplicationClaim gracefully", func() {
			// Try to reconcile non-existent claim
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			})

			// Should not error for missing resource
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("Should update status with ready condition", func() {
			// Create ApplicationClaim
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-status",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "staging",
					Owner: platformv1.OwnerSpec{
						Team:  "Status Team",
						Email: "status@example.com",
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))

			// Check status was updated
			updatedClaim := &platformv1.ApplicationClaim{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      claim.Name,
				Namespace: claim.Namespace,
			}, updatedClaim)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedClaim.Status.Ready).To(BeTrue())
			Expect(updatedClaim.Status.ApplicationsReady).To(BeTrue())
			Expect(updatedClaim.Status.ComponentsReady).To(BeTrue())
		})

		It("Should handle multiple applications in a single claim", func() {
			// Create ApplicationClaim with multiple apps
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-multi",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "development",
					Owner: platformv1.OwnerSpec{
						Team:  "Multi Team",
						Email: "multi@example.com",
					},
					Applications: []platformv1.ApplicationSpec{
						{
							Name:    "frontend",
							Version: "v2.0.0",
							Replicas: func() *int32 {
								r := int32(2)
								return &r
							}(),
							Ports: []platformv1.PortSpec{
								{
									Name:     "http",
									Port:     3000,
									Protocol: "TCP",
								},
							},
						},
						{
							Name:    "backend",
							Version: "v1.5.0",
							Replicas: func() *int32 {
								r := int32(3)
								return &r
							}(),
							Ports: []platformv1.PortSpec{
								{
									Name:     "http",
									Port:     8080,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Create namespace first
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			}
			Expect(fakeClient.Create(ctx, namespace)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))

			// Check both deployments were created
			frontendDep := &appsv1.Deployment{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "frontend",
				Namespace: testNamespace,
			}, frontendDep)
			Expect(err).NotTo(HaveOccurred())
			Expect(*frontendDep.Spec.Replicas).To(Equal(int32(2)))

			backendDep := &appsv1.Deployment{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "backend",
				Namespace: testNamespace,
			}, backendDep)
			Expect(err).NotTo(HaveOccurred())
			Expect(*backendDep.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Context("When handling component resources", func() {
		It("Should deploy PostgreSQL component", func() {
			// Create ApplicationClaim with PostgreSQL
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-postgres",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "production",
					Owner: platformv1.OwnerSpec{
						Team:  "Database Team",
						Email: "db@example.com",
					},
					Components: []platformv1.ComponentSpec{
						{
							Type:    "postgresql",
							Name:    "main-db",
							Version: "14",
						},
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))
		})

		It("Should deploy Redis component", func() {
			// Create ApplicationClaim with Redis
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-redis",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "staging",
					Owner: platformv1.OwnerSpec{
						Team:  "Cache Team",
						Email: "cache@example.com",
					},
					Components: []platformv1.ComponentSpec{
						{
							Type:    "redis",
							Name:    "cache",
							Version: "7.0",
						},
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Verify reconciliation succeeded
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{RequeueAfter: 30 * time.Second}))
		})
	})

	Context("When handling errors", func() {
		It("Should handle deployment creation errors gracefully", func() {
			// Create ApplicationClaim with invalid configuration
			claim := &platformv1.ApplicationClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim-error",
					Namespace: "default",
				},
				Spec: platformv1.ApplicationClaimSpec{
					Namespace:   testNamespace,
					Environment: "production",
					Owner: platformv1.OwnerSpec{
						Team:  "Error Team",
						Email: "error@example.com",
					},
					Applications: []platformv1.ApplicationSpec{
						{
							Name:    "invalid-app",
							Version: "", // Invalid: empty version
							Replicas: func() *int32 {
								r := int32(-1) // Invalid: negative replicas
								return &r
							}(),
						},
					},
				},
			}

			// Create the claim
			Expect(fakeClient.Create(ctx, claim)).To(Succeed())

			// Reconcile should handle error gracefully
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claim.Name,
					Namespace: claim.Namespace,
				},
			})

			// Should still not crash
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

// Unit tests for helper functions
func TestEnsureNamespace(t *testing.T) {
	tests := []struct {
		name          string
		namespaceName string
		existingNS    bool
		wantErr       bool
	}{
		{
			name:          "Create new namespace",
			namespaceName: "new-namespace",
			existingNS:    false,
			wantErr:       false,
		},
		{
			name:          "Namespace already exists",
			namespaceName: "existing-namespace",
			existingNS:    true,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			s := scheme.Scheme
			_ = platformv1.AddToScheme(s)

			fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

			if tt.existingNS {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.namespaceName,
					},
				}
				_ = fakeClient.Create(ctx, ns)
			}

			reconciler := &ApplicationClaimReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Test
			err := reconciler.ensureNamespace(ctx, tt.namespaceName)

			// Verify
			if (err != nil) != tt.wantErr {
				t.Errorf("ensureNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check namespace exists
			ns := &corev1.Namespace{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: tt.namespaceName}, ns)
			if err != nil {
				t.Errorf("Failed to get namespace: %v", err)
			}
		})
	}
}

func TestCreateDeployment(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		app       platformv1.ApplicationSpec
		wantErr   bool
	}{
		{
			name:      "Create valid deployment",
			namespace: "test-ns",
			app: platformv1.ApplicationSpec{
				Name:    "test-app",
				Version: "v1.0.0",
				Replicas: func() *int32 {
					r := int32(3)
					return &r
				}(),
				Ports: []platformv1.PortSpec{
					{
						Name:     "http",
						Port:     8080,
						Protocol: "TCP",
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "Create deployment with environment variables",
			namespace: "test-ns",
			app: platformv1.ApplicationSpec{
				Name:    "test-app-env",
				Version: "v1.0.0",
				Replicas: func() *int32 {
					r := int32(1)
					return &r
				}(),
				Env: []platformv1.EnvVar{
					{
						Name:  "ENV_VAR",
						Value: "test-value",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			s := scheme.Scheme
			_ = platformv1.AddToScheme(s)

			fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

			reconciler := &ApplicationClaimReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Test
			err := reconciler.createDeployment(ctx, tt.namespace, tt.app)

			// Verify
			if (err != nil) != tt.wantErr {
				t.Errorf("createDeployment() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check deployment was created
			if !tt.wantErr {
				deployment := &appsv1.Deployment{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      tt.app.Name,
					Namespace: tt.namespace,
				}, deployment)
				if err != nil {
					t.Errorf("Failed to get deployment: %v", err)
				}

				// Verify deployment spec
				if *deployment.Spec.Replicas != tt.app.Replicas {
					t.Errorf("Replicas mismatch: got %d, want %d",
						*deployment.Spec.Replicas, tt.app.Replicas)
				}

				// Verify environment variables
				if len(tt.app.Env) > 0 {
					container := deployment.Spec.Template.Spec.Containers[0]
					if len(container.Env) != len(tt.app.Env) {
						t.Errorf("Env vars mismatch: got %d, want %d",
							len(container.Env), len(tt.app.Env))
					}
				}
			}
		})
	}
}

func TestCreateService(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		app       platformv1.ApplicationSpec
		wantErr   bool
	}{
		{
			name:      "Create service with single port",
			namespace: "test-ns",
			app: platformv1.ApplicationSpec{
				Name:    "test-service",
				Version: "v1.0.0",
				Ports: []platformv1.PortSpec{
					{
						Name:     "http",
						Port:     8080,
						Protocol: "TCP",
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "Create service with multiple ports",
			namespace: "test-ns",
			app: platformv1.ApplicationSpec{
				Name:    "multi-port-service",
				Version: "v1.0.0",
				Ports: []platformv1.PortSpec{
					{
						Name:     "http",
						Port:     8080,
						Protocol: "TCP",
					},
					{
						Name:     "metrics",
						Port:     9090,
						Protocol: "TCP",
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "Create service without ports",
			namespace: "test-ns",
			app: platformv1.ApplicationSpec{
				Name:    "no-port-service",
				Version: "v1.0.0",
				Ports:   []platformv1.PortSpec{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			s := scheme.Scheme
			_ = platformv1.AddToScheme(s)

			fakeClient := fake.NewClientBuilder().WithScheme(s).Build()

			reconciler := &ApplicationClaimReconciler{
				Client: fakeClient,
				Scheme: s,
			}

			// Test
			err := reconciler.createService(ctx, tt.namespace, tt.app)

			// Verify
			if (err != nil) != tt.wantErr {
				t.Errorf("createService() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check service was created
			if !tt.wantErr && len(tt.app.Ports) > 0 {
				service := &corev1.Service{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Name:      tt.app.Name,
					Namespace: tt.namespace,
				}, service)
				if err != nil {
					t.Errorf("Failed to get service: %v", err)
				}

				// Verify service ports
				if len(service.Spec.Ports) != len(tt.app.Ports) {
					t.Errorf("Port count mismatch: got %d, want %d",
						len(service.Spec.Ports), len(tt.app.Ports))
				}

				// Verify each port
				for i, port := range tt.app.Ports {
					if service.Spec.Ports[i].Port != port.Port {
						t.Errorf("Port mismatch at index %d: got %d, want %d",
							i, service.Spec.Ports[i].Port, port.Port)
					}
				}
			}
		})
	}
}

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
)

const (
	testTimeout   = 5 * time.Minute
	pollInterval  = 5 * time.Second
	testNamespace = "integration-test"
)

func TestApplicationClaimIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup Kubernetes client
	cfg, err := config.GetConfig()
	if err != nil {
		t.Fatalf("Failed to get config: %v", err)
	}

	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	t.Run("CreateApplicationClaim", func(t *testing.T) {
		testCreateApplicationClaim(t, ctx, k8sClient)
	})

	t.Run("DeployBackendService", func(t *testing.T) {
		testDeployBackendService(t, ctx, k8sClient)
	})

	t.Run("ScaleApplication", func(t *testing.T) {
		testScaleApplication(t, ctx, k8sClient)
	})

	t.Run("UpdateApplication", func(t *testing.T) {
		testUpdateApplication(t, ctx, k8sClient)
	})

	t.Run("DeleteApplicationClaim", func(t *testing.T) {
		testDeleteApplicationClaim(t, ctx, k8sClient)
	})

	t.Run("ReconciliationLoop", func(t *testing.T) {
		testReconciliationLoop(t, ctx, k8sClient)
	})

	t.Run("ComponentDeployment", func(t *testing.T) {
		testComponentDeployment(t, ctx, k8sClient)
	})

	t.Run("MultiApplicationDeployment", func(t *testing.T) {
		testMultiApplicationDeployment(t, ctx, k8sClient)
	})
}

func testCreateApplicationClaim(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create test namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNamespace,
		},
	}
	if err := k8sClient.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("Failed to create namespace: %v", err)
	}

	// Create ApplicationClaim
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-create",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "development",
			Owner: platformv1.OwnerSpec{
				Team:  "Integration Test",
				Email: "test@example.com",
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for claim to be ready
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		updatedClaim := &platformv1.ApplicationClaim{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		}, updatedClaim); err != nil {
			return false, err
		}
		return updatedClaim.Status.Ready, nil
	})

	if err != nil {
		t.Fatalf("ApplicationClaim did not become ready: %v", err)
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testDeployBackendService(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim with backend service
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-backend",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "staging",
			Owner: platformv1.OwnerSpec{
				Team:  "Backend Team",
				Email: "backend@example.com",
			},
			Applications: []platformv1.ApplicationSpec{
				{
					Name:    "backend-service",
					Version: "v1.0.0",
					Replicas: func() *int32 {
						r := int32(2)
						return &r
					}(),
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
							Value: "staging",
						},
					},
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for deployment to be created
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "backend-service",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return *deployment.Spec.Replicas == 2, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not created: %v", err)
	}

	// Verify service was created
	service := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      "backend-service",
		Namespace: testNamespace,
	}, service); err != nil {
		t.Fatalf("Service was not created: %v", err)
	}

	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 8080 {
		t.Errorf("Service ports mismatch")
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testScaleApplication(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim
	initialReplicas := int32(1)
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-scale",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "development",
			Owner: platformv1.OwnerSpec{
				Team:  "Scale Team",
				Email: "scale@example.com",
			},
			Applications: []platformv1.ApplicationSpec{
				{
					Name:     "scalable-app",
					Version:  "v1.0.0",
					Replicas: &initialReplicas,
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for initial deployment
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "scalable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return *deployment.Spec.Replicas == initialReplicas, nil
	})

	if err != nil {
		t.Fatalf("Initial deployment was not created: %v", err)
	}

	// Update replicas
	updatedClaim := &platformv1.ApplicationClaim{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      claim.Name,
		Namespace: claim.Namespace,
	}, updatedClaim); err != nil {
		t.Fatalf("Failed to get ApplicationClaim: %v", err)
	}

	newReplicas := int32(3)
	updatedClaim.Spec.Applications[0].Replicas = &newReplicas
	if err := k8sClient.Update(ctx, updatedClaim); err != nil {
		t.Fatalf("Failed to update ApplicationClaim: %v", err)
	}

	// Wait for deployment to scale
	err = wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "scalable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			return false, err
		}
		return *deployment.Spec.Replicas == newReplicas, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not scaled: %v", err)
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, updatedClaim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testUpdateApplication(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-update",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "development",
			Owner: platformv1.OwnerSpec{
				Team:  "Update Team",
				Email: "update@example.com",
			},
			Applications: []platformv1.ApplicationSpec{
				{
					Name:    "updatable-app",
					Version: "v1.0.0",
					Replicas: func() *int32 {
						r := int32(1)
						return &r
					}(),
					Env: []platformv1.EnvVar{
						{
							Name:  "VERSION",
							Value: "1.0.0",
						},
					},
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for initial deployment
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "updatable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})

	if err != nil {
		t.Fatalf("Initial deployment was not created: %v", err)
	}

	// Update application version
	updatedClaim := &platformv1.ApplicationClaim{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      claim.Name,
		Namespace: claim.Namespace,
	}, updatedClaim); err != nil {
		t.Fatalf("Failed to get ApplicationClaim: %v", err)
	}

	updatedClaim.Spec.Applications[0].Version = "v2.0.0"
	updatedClaim.Spec.Applications[0].Env[0].Value = "2.0.0"
	if err := k8sClient.Update(ctx, updatedClaim); err != nil {
		t.Fatalf("Failed to update ApplicationClaim: %v", err)
	}

	// Wait for deployment to update
	err = wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "updatable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			return false, err
		}

		// Check if image was updated
		if len(deployment.Spec.Template.Spec.Containers) > 0 {
			image := deployment.Spec.Template.Spec.Containers[0].Image
			return image == "updatable-app:v2.0.0", nil
		}
		return false, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not updated: %v", err)
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, updatedClaim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testDeleteApplicationClaim(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-delete",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "development",
			Owner: platformv1.OwnerSpec{
				Team:  "Delete Team",
				Email: "delete@example.com",
			},
			Applications: []platformv1.ApplicationSpec{
				{
					Name:    "deletable-app",
					Version: "v1.0.0",
					Replicas: func() *int32 {
						r := int32(1)
						return &r
					}(),
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for deployment to be created
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "deletable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not created: %v", err)
	}

	// Delete ApplicationClaim
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Fatalf("Failed to delete ApplicationClaim: %v", err)
	}

	// Wait for deployment to be deleted
	err = wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "deletable-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not deleted after ApplicationClaim deletion: %v", err)
	}
}

func testReconciliationLoop(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-reconcile",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "production",
			Owner: platformv1.OwnerSpec{
				Team:  "Reconcile Team",
				Email: "reconcile@example.com",
			},
			Applications: []platformv1.ApplicationSpec{
				{
					Name:    "reconcile-app",
					Version: "v1.0.0",
					Replicas: func() *int32 {
						r := int32(2)
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

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for deployment to be created
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "reconcile-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not created: %v", err)
	}

	// Manually delete the deployment (simulating drift)
	deployment := &appsv1.Deployment{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      "reconcile-app",
		Namespace: testNamespace,
	}, deployment); err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}

	if err := k8sClient.Delete(ctx, deployment); err != nil {
		t.Fatalf("Failed to delete deployment: %v", err)
	}

	// Wait for deployment to be recreated by reconciliation
	err = wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		deployment := &appsv1.Deployment{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      "reconcile-app",
			Namespace: testNamespace,
		}, deployment); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return *deployment.Spec.Replicas == 2, nil
	})

	if err != nil {
		t.Fatalf("Deployment was not recreated by reconciliation: %v", err)
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testComponentDeployment(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim with components
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-components",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "production",
			Owner: platformv1.OwnerSpec{
				Team:  "Component Team",
				Email: "components@example.com",
			},
			Components: []platformv1.ComponentSpec{
				{
					Type:    "postgresql",
					Name:    "test-db",
					Version: "14",
				},
				{
					Type:    "redis",
					Name:    "test-cache",
					Version: "7.0",
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for claim to be ready
	err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
		updatedClaim := &platformv1.ApplicationClaim{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		}, updatedClaim); err != nil {
			return false, err
		}
		return updatedClaim.Status.ComponentsReady, nil
	})

	if err != nil {
		t.Fatalf("Components were not deployed: %v", err)
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

func testMultiApplicationDeployment(t *testing.T, ctx context.Context, k8sClient client.Client) {
	// Create ApplicationClaim with multiple applications
	claim := &platformv1.ApplicationClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-claim-multi",
			Namespace: "default",
		},
		Spec: platformv1.ApplicationClaimSpec{
			Namespace:   testNamespace,
			Environment: "production",
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
				{
					Name:    "worker",
					Version: "v1.0.0",
					Replicas: func() *int32 {
						r := int32(1)
						return &r
					}(),
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, claim); err != nil {
		t.Fatalf("Failed to create ApplicationClaim: %v", err)
	}

	// Wait for all deployments to be created
	apps := []string{"frontend", "backend", "worker"}
	expectedReplicas := map[string]int32{
		"frontend": 2,
		"backend":  3,
		"worker":   1,
	}

	for _, appName := range apps {
		err := wait.PollImmediate(pollInterval, testTimeout, func() (bool, error) {
			deployment := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      appName,
				Namespace: testNamespace,
			}, deployment); err != nil {
				if errors.IsNotFound(err) {
					return false, nil
				}
				return false, err
			}
			return *deployment.Spec.Replicas == expectedReplicas[appName], nil
		})

		if err != nil {
			t.Fatalf("Deployment %s was not created: %v", appName, err)
		}
	}

	// Verify services for apps with ports
	for _, appName := range []string{"frontend", "backend"} {
		service := &corev1.Service{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      appName,
			Namespace: testNamespace,
		}, service); err != nil {
			t.Errorf("Service %s was not created: %v", appName, err)
		}
	}

	// Cleanup
	if err := k8sClient.Delete(ctx, claim); err != nil {
		t.Errorf("Failed to delete ApplicationClaim: %v", err)
	}
}

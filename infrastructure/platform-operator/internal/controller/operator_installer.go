package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

// Operator metadata for auto-installation
type OperatorInfo struct {
	Name        string
	Namespace   string
	HelmRepo    string
	HelmChart   string
	Version     string
	Description string
}

var operatorRegistry = map[string]OperatorInfo{
	"postgresql": {
		Name:        "cloudnative-pg",
		Namespace:   "cnpg-system",
		HelmRepo:    "https://cloudnative-pg.github.io/charts",
		HelmChart:   "cloudnative-pg",
		Version:     "0.20.0",
		Description: "CloudNativePG Operator for PostgreSQL",
	},
	"redis": {
		Name:        "redis-operator",
		Namespace:   "redis-operator",
		HelmRepo:    "https://spotahome.github.io/redis-operator",
		HelmChart:   "redis-operator",
		Version:     "3.2.9",
		Description: "Redis Operator by Spotahome",
	},
	"rabbitmq": {
		Name:        "rabbitmq-cluster-operator",
		Namespace:   "rabbitmq-system",
		HelmRepo:    "https://charts.bitnami.com/bitnami",
		HelmChart:   "rabbitmq-cluster-operator",
		Version:     "4.0.0",
		Description: "RabbitMQ Cluster Operator",
	},
	"mongodb": {
		Name:        "mongodb-community-operator",
		Namespace:   "mongodb",
		HelmRepo:    "https://mongodb.github.io/helm-charts",
		HelmChart:   "community-operator",
		Version:     "0.9.0",
		Description: "MongoDB Community Operator",
	},
	"elasticsearch": {
		Name:        "elastic-operator",
		Namespace:   "elastic-system",
		HelmRepo:    "https://helm.elastic.co",
		HelmChart:   "eck-operator",
		Version:     "2.11.0",
		Description: "Elastic Cloud on Kubernetes (ECK) Operator",
	},
}

// detectRequiredOperators detects which operators are needed based on claim components
func (r *ApplicationClaimReconciler) detectRequiredOperators(claim *platformv1.ApplicationClaim) []string {
	logger := log.FromContext(context.Background())

	operatorsNeeded := make(map[string]bool)

	for _, component := range claim.Spec.Components {
		if _, exists := operatorRegistry[component.Type]; exists {
			operatorsNeeded[component.Type] = true
			logger.Info("Detected required operator", "type", component.Type)
		}
	}

	result := make([]string, 0, len(operatorsNeeded))
	for opType := range operatorsNeeded {
		result = append(result, opType)
	}

	return result
}

// ensureOperatorsInstalled ensures all required operators are installed
func (r *ApplicationClaimReconciler) ensureOperatorsInstalled(ctx context.Context, claim *platformv1.ApplicationClaim) error {
	logger := log.FromContext(ctx)

	requiredOperators := r.detectRequiredOperators(claim)
	if len(requiredOperators) == 0 {
		logger.Info("No operators required for this claim")
		return nil
	}

	logger.Info("Ensuring operators are installed", "operators", requiredOperators)

	for _, opType := range requiredOperators {
		opInfo := operatorRegistry[opType]

		// Check if operator is already installed
		installed, err := r.isOperatorInstalled(ctx, opInfo)
		if err != nil {
			return fmt.Errorf("failed to check if operator %s is installed: %w", opInfo.Name, err)
		}

		if !installed {
			logger.Info("Installing operator", "operator", opInfo.Name)
			if err := r.installOperator(ctx, opInfo); err != nil {
				return fmt.Errorf("failed to install operator %s: %w", opInfo.Name, err)
			}
			logger.Info("Successfully installed operator", "operator", opInfo.Name)
		} else {
			logger.Info("Operator already installed", "operator", opInfo.Name)
		}
	}

	return nil
}

// isOperatorInstalled checks if an operator is already installed
func (r *ApplicationClaimReconciler) isOperatorInstalled(ctx context.Context, opInfo OperatorInfo) (bool, error) {
	// Check if the operator's ArgoCD Application exists
	app := &unstructured.Unstructured{}
	app.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err := r.Get(ctx, types.NamespacedName{
		Name:      opInfo.Name,
		Namespace: "argocd",
	}, app)

	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// installOperator installs an operator via ArgoCD Application
func (r *ApplicationClaimReconciler) installOperator(ctx context.Context, opInfo OperatorInfo) error {
	logger := log.FromContext(ctx)

	// Create ArgoCD Application for the operator
	application := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      opInfo.Name,
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"platform.infraforge.io/managed":  "true",
					"platform.infraforge.io/operator": "true",
					"platform.infraforge.io/type":     opInfo.Name,
				},
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        opInfo.HelmRepo,
					"chart":          opInfo.HelmChart,
					"targetRevision": opInfo.Version,
					"helm": map[string]interface{}{
						"releaseName": opInfo.Name,
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": opInfo.Namespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
					"syncOptions": []string{
						"CreateNamespace=true",
						"ServerSideApply=true",
					},
				},
			},
		},
	}

	application.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	logger.Info("Creating ArgoCD Application for operator", "operator", opInfo.Name)
	if err := r.Create(ctx, application); err != nil {
		return fmt.Errorf("failed to create ArgoCD Application: %w", err)
	}

	return nil
}

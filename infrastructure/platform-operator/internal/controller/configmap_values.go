package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1 "github.com/infraforge/platform-operator/api/v1"
)

// storeValuesInConfigMap stores generated Helm values in a ConfigMap
func (r *ApplicationClaimReconciler) storeValuesInConfigMap(ctx context.Context, claim *platformv1.ApplicationClaim, appName, valuesYAML string) error {
	logger := log.FromContext(ctx)

	cmName := fmt.Sprintf("%s-%s-values", claim.Name, appName)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: "argocd", // Store in argocd namespace
			Labels: map[string]string{
				"platform.infraforge.io/managed": "true",
				"platform.infraforge.io/claim":   claim.Name,
				"platform.infraforge.io/app":     appName,
			},
		},
		Data: map[string]string{
			"values.yaml": valuesYAML,
		},
	}

	// Check if ConfigMap exists
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      cmName,
		Namespace: "argocd",
	}, existing)

	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ConfigMap
			logger.Info("Creating values ConfigMap", "name", cmName)
			if err := r.Create(ctx, cm); err != nil {
				return fmt.Errorf("failed to create ConfigMap: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update existing ConfigMap
	logger.Info("Updating values ConfigMap", "name", cmName)
	existing.Data = cm.Data
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	return nil
}

package reconcile

import (
	"context"
	"fmt"

	"github.com/authzed/controller-idioms/handler"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ManagedLabel is the label applied to adopted resources
	ManagedLabel = "pequod.io/managed"

	// OwnerAnnotationPrefix is the prefix for owner annotations
	OwnerAnnotationPrefix = "pequod.io/owner-"
)

// AdoptResourcesHandler adopts Secrets and ConfigMaps referenced in WebService.Spec.EnvFrom
// It adds labels and annotations to mark them as managed by this controller
type AdoptResourcesHandler struct {
	client client.Client
	next   handler.Handler
}

// NewAdoptResourcesHandler creates a new adoption handler
func NewAdoptResourcesHandler(client client.Client, next handler.Handler) *AdoptResourcesHandler {
	return &AdoptResourcesHandler{
		client: client,
		next:   next,
	}
}

// Handle adopts all Secrets and ConfigMaps referenced in the WebService
func (h *AdoptResourcesHandler) Handle(ctx context.Context) {
	logger := log.FromContext(ctx)

	ws, ok := CtxWebService.Value(ctx)
	if !ok {
		logger.Error(nil, "WebService not found in context")
		// Get queue operations and requeue with error
		if queueOps, ok := CtxQueue.Value(ctx); ok {
			queueOps.RequeueErr(fmt.Errorf("WebService not found in context"))
		}
		return
	}

	ownerKey := fmt.Sprintf("%s/%s", ws.Namespace, ws.Name)
	ownerAnnotation := OwnerAnnotationPrefix + ws.Name

	// Adopt each Secret and ConfigMap referenced in envFrom
	for _, envFrom := range ws.Spec.EnvFrom {
		if envFrom.SecretRef != nil {
			if err := h.adoptSecret(ctx, ws.Namespace, envFrom.SecretRef.Name, ownerKey, ownerAnnotation); err != nil {
				logger.Error(err, "failed to adopt Secret", "secret", envFrom.SecretRef.Name)
				// Get queue operations and requeue with error
				if queueOps, ok := CtxQueue.Value(ctx); ok {
					queueOps.RequeueErr(err)
				}
				return
			}
		}

		if envFrom.ConfigMapRef != nil {
			if err := h.adoptConfigMap(ctx, ws.Namespace, envFrom.ConfigMapRef.Name, ownerKey, ownerAnnotation); err != nil {
				logger.Error(err, "failed to adopt ConfigMap", "configmap", envFrom.ConfigMapRef.Name)
				// Get queue operations and requeue with error
				if queueOps, ok := CtxQueue.Value(ctx); ok {
					queueOps.RequeueErr(err)
				}
				return
			}
		}
	}

	h.next.Handle(ctx)
}

// adoptSecret adopts a Secret by adding labels and annotations
func (h *AdoptResourcesHandler) adoptSecret(ctx context.Context, namespace, name, ownerKey, ownerAnnotation string) error {
	logger := log.FromContext(ctx)

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := h.client.Get(ctx, key, secret); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("Secret %s not found", name)
		}
		return err
	}

	// Check if already adopted
	if secret.Labels[ManagedLabel] == "true" && secret.Annotations[ownerAnnotation] == "owned" {
		logger.V(2).Info("Secret already adopted", "secret", name)
		return nil
	}

	// Add labels and annotations
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	secret.Labels[ManagedLabel] = "true"

	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations[ownerAnnotation] = "owned"

	// Update the Secret
	if err := h.client.Update(ctx, secret); err != nil {
		return fmt.Errorf("failed to update Secret %s: %w", name, err)
	}

	logger.V(1).Info("adopted Secret", "secret", name, "owner", ownerKey)
	return nil
}

// adoptConfigMap adopts a ConfigMap by adding labels and annotations
func (h *AdoptResourcesHandler) adoptConfigMap(ctx context.Context, namespace, name, ownerKey, ownerAnnotation string) error {
	logger := log.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: name, Namespace: namespace}

	if err := h.client.Get(ctx, key, configMap); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("ConfigMap %s not found", name)
		}
		return err
	}

	// Check if already adopted
	if configMap.Labels[ManagedLabel] == "true" && configMap.Annotations[ownerAnnotation] == "owned" {
		logger.V(2).Info("ConfigMap already adopted", "configmap", name)
		return nil
	}

	// Add labels and annotations
	if configMap.Labels == nil {
		configMap.Labels = make(map[string]string)
	}
	configMap.Labels[ManagedLabel] = "true"

	if configMap.Annotations == nil {
		configMap.Annotations = make(map[string]string)
	}
	configMap.Annotations[ownerAnnotation] = "owned"

	// Update the ConfigMap
	if err := h.client.Update(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %w", name, err)
	}

	logger.V(1).Info("adopted ConfigMap", "configmap", name, "owner", ownerKey)
	return nil
}

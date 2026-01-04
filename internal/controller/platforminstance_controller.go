/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/platformloader"
	"github.com/chazu/pequod/pkg/reconcile"
)

// PlatformInstanceReconciler reconciles instances of dynamically generated CRDs.
// When a Transform generates a CRD (e.g., WebService), users can create instances
// of that CRD. This controller watches those instances and creates ResourceGraphs.
type PlatformInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Renderer *platformloader.Renderer

	// handler contains the reconciliation logic
	handler *reconcile.InstanceHandlers

	// watchedGVKs tracks which GVKs we're watching
	watchedGVKs map[schema.GroupVersionKind]bool
	watchMutex  sync.RWMutex

	// controller is the underlying controller for dynamic watch management
	ctrl controller.Controller

	// mgr is the manager for cache access
	mgr ctrl.Manager
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.pequod.io,resources=resourcegraphs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.pequod.io,resources=resourcegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.pequod.io,resources=transforms,verbs=get;list;watch

// Reconcile handles platform instance resources (e.g., WebService instances)
func (r *PlatformInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// We need to determine the GVK from the request
	// The request only contains namespace/name, so we need to find the matching instance
	// by checking all watched GVKs

	r.watchMutex.RLock()
	gvks := make([]schema.GroupVersionKind, 0, len(r.watchedGVKs))
	for gvk := range r.watchedGVKs {
		gvks = append(gvks, gvk)
	}
	r.watchMutex.RUnlock()

	// Try to get the instance using each watched GVK
	var instance *unstructured.Unstructured
	var instanceGVK schema.GroupVersionKind

	for _, gvk := range gvks {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)

		if err := r.Get(ctx, req.NamespacedName, u); err == nil {
			instance = u
			instanceGVK = gvk
			break
		}
	}

	if instance == nil {
		// Object not found or deleted
		logger.V(1).Info("Instance not found", "request", req)
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling platform instance",
		"gvk", instanceGVK.String(),
		"name", instance.GetName(),
		"namespace", instance.GetNamespace())

	// Find the Transform that generated this CRD
	transform, err := reconcile.FindTransformForGVK(ctx, r.Client, instanceGVK)
	if err != nil {
		logger.Error(err, "Failed to find Transform for GVK", "gvk", instanceGVK.String())
		return ctrl.Result{}, err
	}

	// Delegate to the handler
	return r.handler.Reconcile(ctx, instance, transform)
}

// SetupWithManager sets up the controller with the Manager
func (r *PlatformInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.mgr = mgr
	r.watchedGVKs = make(map[schema.GroupVersionKind]bool)

	// Initialize the handler
	r.handler = reconcile.NewInstanceHandlers(
		r.Client,
		r.Scheme,
		r.Recorder,
		r.Renderer,
	)

	// Build the controller
	c, err := ctrl.NewControllerManagedBy(mgr).
		Named("platforminstance").
		// We don't use For() since we watch dynamic types
		// Instead, we'll add watches dynamically
		Build(r)
	if err != nil {
		return err
	}

	r.ctrl = c

	// Add the discovery loop as a runnable to the manager
	// This ensures proper lifecycle management
	if err := mgr.Add(&watchDiscoveryRunnable{reconciler: r}); err != nil {
		return err
	}

	return nil
}

// watchDiscoveryRunnable implements manager.Runnable for the watch discovery loop
type watchDiscoveryRunnable struct {
	reconciler *PlatformInstanceReconciler
}

// Start implements manager.Runnable
func (w *watchDiscoveryRunnable) Start(ctx context.Context) error {
	w.reconciler.watchDiscoveryLoop(ctx)
	return nil
}

// watchDiscoveryLoop periodically checks for new Transforms and adds watches
func (r *PlatformInstanceReconciler) watchDiscoveryLoop(ctx context.Context) {
	logger := logf.FromContext(ctx).WithName("watch-discovery")

	// Wait for cache to sync
	if !r.mgr.GetCache().WaitForCacheSync(ctx) {
		logger.Error(nil, "Failed to wait for cache sync")
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Initial discovery
	r.discoverAndWatchPlatformTypes(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.discoverAndWatchPlatformTypes(ctx)
		}
	}
}

// discoverAndWatchPlatformTypes finds all Transforms with generated CRDs and adds watches
func (r *PlatformInstanceReconciler) discoverAndWatchPlatformTypes(ctx context.Context) {
	logger := logf.FromContext(ctx).WithName("watch-discovery")

	// List all Transforms
	transforms := &platformv1alpha1.TransformList{}
	if err := r.List(ctx, transforms); err != nil {
		logger.Error(err, "Failed to list Transforms")
		return
	}

	for _, tf := range transforms.Items {
		if tf.Status.GeneratedCRD == nil {
			continue
		}

		// Parse the GVK from the generated CRD reference
		gv, err := schema.ParseGroupVersion(tf.Status.GeneratedCRD.APIVersion)
		if err != nil {
			logger.Error(err, "Failed to parse GeneratedCRD APIVersion",
				"transform", tf.Name,
				"apiVersion", tf.Status.GeneratedCRD.APIVersion)
			continue
		}

		gvk := schema.GroupVersionKind{
			Group:   gv.Group,
			Version: gv.Version,
			Kind:    tf.Status.GeneratedCRD.Kind,
		}

		// Check if we're already watching this GVK
		r.watchMutex.RLock()
		watching := r.watchedGVKs[gvk]
		r.watchMutex.RUnlock()

		if watching {
			continue
		}

		// Add watch for this GVK
		if err := r.addWatch(ctx, gvk); err != nil {
			logger.Error(err, "Failed to add watch for GVK", "gvk", gvk.String())
			continue
		}

		r.watchMutex.Lock()
		r.watchedGVKs[gvk] = true
		r.watchMutex.Unlock()

		logger.Info("Added watch for platform type", "gvk", gvk.String())
	}
}

// addWatch adds a watch for the given GVK
func (r *PlatformInstanceReconciler) addWatch(ctx context.Context, gvk schema.GroupVersionKind) error {
	// Create an unstructured object for this GVK
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)

	// Add the watch using source.Kind
	// We use TypedEnqueueRequestsFromMapFunc for unstructured objects
	return r.ctrl.Watch(
		source.Kind(
			r.mgr.GetCache(),
			u,
			handler.TypedEnqueueRequestsFromMapFunc(
				func(ctx context.Context, obj *unstructured.Unstructured) []ctrl.Request {
					return []ctrl.Request{
						{NamespacedName: client.ObjectKeyFromObject(obj)},
					}
				},
			),
		),
	)
}

// RemoveWatch removes a watch for the given GVK (called when Transform is deleted)
func (r *PlatformInstanceReconciler) RemoveWatch(gvk schema.GroupVersionKind) {
	r.watchMutex.Lock()
	defer r.watchMutex.Unlock()
	delete(r.watchedGVKs, gvk)
	// Note: controller-runtime doesn't support removing watches dynamically
	// The watch will remain but instances will 404 since the CRD is deleted
}

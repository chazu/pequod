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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
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

	// instanceGVKIndex maps NamespacedName to GVK for O(1) lookup during reconcile.
	// This avoids iterating through all watched GVKs and making API calls.
	instanceGVKIndex map[types.NamespacedName]schema.GroupVersionKind
	indexMutex       sync.RWMutex

	// controller is the underlying controller for dynamic watch management
	ctrl controller.Controller

	// mgr is the manager for cache access
	mgr ctrl.Manager
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pequod.io,resources=resourcegraphs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pequod.io,resources=resourcegraphs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pequod.io,resources=transforms,verbs=get;list;watch

// Reconcile handles platform instance resources (e.g., WebService instances)
func (r *PlatformInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var instance *unstructured.Unstructured
	var instanceGVK schema.GroupVersionKind

	// First, try O(1) lookup from the GVK index
	r.indexMutex.RLock()
	cachedGVK, found := r.instanceGVKIndex[req.NamespacedName]
	r.indexMutex.RUnlock()

	if found {
		// Use the cached GVK for a direct lookup
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(cachedGVK)

		if err := r.Get(ctx, req.NamespacedName, u); err == nil {
			instance = u
			instanceGVK = cachedGVK
		} else {
			// Object not found - remove stale index entry
			r.indexMutex.Lock()
			delete(r.instanceGVKIndex, req.NamespacedName)
			r.indexMutex.Unlock()
			logger.V(1).Info("Instance not found (removed from index)", "request", req)
			return ctrl.Result{}, nil
		}
	} else {
		// Fallback: iterate through watched GVKs (should rarely happen)
		// This handles cases where the watch event hasn't populated the index yet
		r.watchMutex.RLock()
		gvks := make([]schema.GroupVersionKind, 0, len(r.watchedGVKs))
		for gvk := range r.watchedGVKs {
			gvks = append(gvks, gvk)
		}
		r.watchMutex.RUnlock()

		for _, gvk := range gvks {
			u := &unstructured.Unstructured{}
			u.SetGroupVersionKind(gvk)

			if err := r.Get(ctx, req.NamespacedName, u); err == nil {
				instance = u
				instanceGVK = gvk

				// Populate the index for future lookups
				r.indexMutex.Lock()
				r.instanceGVKIndex[req.NamespacedName] = gvk
				r.indexMutex.Unlock()
				break
			}
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
	r.instanceGVKIndex = make(map[types.NamespacedName]schema.GroupVersionKind)

	// Initialize the handler
	r.handler = reconcile.NewInstanceHandlers(
		r.Client,
		r.Scheme,
		r.Recorder,
		r.Renderer,
	)

	// Build the controller with a watch on Transforms
	// When a Transform's status is updated with a GeneratedCRD, we add a watch for that CRD type
	c, err := ctrl.NewControllerManagedBy(mgr).
		Named("platforminstance").
		// Watch Transforms to add watches when CRDs are generated (event-driven, not polling)
		Watches(
			&platformv1alpha1.Transform{},
			handler.EnqueueRequestsFromMapFunc(r.handleTransformChange),
		).
		Build(r)
	if err != nil {
		return err
	}

	r.ctrl = c

	// Add a startup runnable for initial discovery of existing Transforms
	// This handles Transforms that existed before the controller started
	if err := mgr.Add(&initialDiscoveryRunnable{reconciler: r}); err != nil {
		return err
	}

	return nil
}

// initialDiscoveryRunnable handles one-time startup discovery for existing Transforms
type initialDiscoveryRunnable struct {
	reconciler *PlatformInstanceReconciler
}

// Start implements manager.Runnable
func (r *initialDiscoveryRunnable) Start(ctx context.Context) error {
	logger := logf.FromContext(ctx).WithName("initial-discovery")

	// Wait for cache to sync before discovery
	if !r.reconciler.mgr.GetCache().WaitForCacheSync(ctx) {
		logger.Error(nil, "Failed to wait for cache sync")
		return nil
	}

	// One-time discovery for Transforms that existed before controller startup
	r.reconciler.discoverAndWatchPlatformTypes(ctx)
	return nil
}

// handleTransformChange processes Transform changes and adds watches for new CRDs.
// This is event-driven - called whenever a Transform is created, updated, or deleted.
func (r *PlatformInstanceReconciler) handleTransformChange(ctx context.Context, obj client.Object) []ctrl.Request {
	logger := logf.FromContext(ctx).WithName("transform-watch")

	tf, ok := obj.(*platformv1alpha1.Transform)
	if !ok {
		return nil
	}

	// Only process Transforms that have generated a CRD
	if tf.Status.GeneratedCRD == nil {
		return nil
	}

	// Parse the GVK from the generated CRD reference
	gv, err := schema.ParseGroupVersion(tf.Status.GeneratedCRD.APIVersion)
	if err != nil {
		logger.Error(err, "Failed to parse GeneratedCRD APIVersion",
			"transform", tf.Name,
			"apiVersion", tf.Status.GeneratedCRD.APIVersion)
		return nil
	}

	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    tf.Status.GeneratedCRD.Kind,
	}

	// Fast path: check if already watching (read lock)
	r.watchMutex.RLock()
	watching := r.watchedGVKs[gvk]
	r.watchMutex.RUnlock()

	if watching {
		return nil
	}

	// Ensure the CRD is established before adding a watch
	// This prevents errors when the Transform status is updated before the CRD is ready
	crdName := tf.Status.GeneratedCRD.Name
	if !r.isCRDEstablished(ctx, crdName) {
		logger.V(1).Info("CRD not yet established, skipping watch", "gvk", gvk.String(), "crd", crdName)
		return nil
	}

	// Add watch for this GVK
	if err := r.addWatch(ctx, gvk); err != nil {
		logger.Error(err, "Failed to add watch for GVK", "gvk", gvk.String())
		return nil
	}

	r.watchMutex.Lock()
	r.watchedGVKs[gvk] = true
	r.watchMutex.Unlock()

	logger.Info("Added watch for platform type", "gvk", gvk.String(), "transform", tf.Name)

	// Return nil - we don't need to reconcile anything specific.
	// The new watch will trigger reconciles for existing instances of this CRD.
	return nil
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

		// Ensure the CRD is established before adding a watch
		crdName := tf.Status.GeneratedCRD.Name
		if !r.isCRDEstablished(ctx, crdName) {
			logger.V(1).Info("CRD not yet established, skipping watch", "gvk", gvk.String(), "crd", crdName)
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
					key := client.ObjectKeyFromObject(obj)
					objGVK := obj.GetObjectKind().GroupVersionKind()

					// Update the GVK index for O(1) lookup during reconcile
					r.indexMutex.Lock()
					r.instanceGVKIndex[key] = objGVK
					r.indexMutex.Unlock()

					return []ctrl.Request{{NamespacedName: key}}
				},
			),
		),
	)
}

// RemoveWatch removes a GVK from the watched set.
// Note: This only removes the GVK from our tracking map. Due to controller-runtime
// limitations, the underlying informer watch cannot be dynamically removed.
// The watch will remain active but will naturally stop receiving events once
// the associated CRD is deleted from the cluster.
func (r *PlatformInstanceReconciler) RemoveWatch(gvk schema.GroupVersionKind) {
	r.watchMutex.Lock()
	defer r.watchMutex.Unlock()
	delete(r.watchedGVKs, gvk)
}

// isCRDEstablished checks if the CRD with the given name exists and is established.
// This prevents attempting to watch a CRD before it's ready to serve requests.
func (r *PlatformInstanceReconciler) isCRDEstablished(ctx context.Context, crdName string) bool {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.Get(ctx, types.NamespacedName{Name: crdName}, crd); err != nil {
		return false
	}

	// Check if the CRD is established
	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
			return true
		}
	}

	return false
}

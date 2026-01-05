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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
	"github.com/chazu/pequod/pkg/platformloader"
	"github.com/chazu/pequod/pkg/reconcile"
)

// TransformReconciler reconciles Transform resources.
// Transform is a platform definition that generates a CRD for developers to use.
// Platform engineers create Transforms, which generate CRDs (e.g., WebService, Database).
type TransformReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	PlatformLoader *platformloader.Loader
	Recorder       record.EventRecorder

	// Handler-based reconciler
	reconciler *reconcile.TransformReconciler
}

// +kubebuilder:rbac:groups=platform.platform.example.com,resources=transforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=transforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=transforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles Transform resources
func (r *TransformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling Transform", "name", req.Name, "namespace", req.Namespace)

	// Delegate to the handler-based reconciler
	return r.reconciler.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager
func (r *TransformReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the handler-based reconciler
	r.reconciler = reconcile.NewTransformReconciler(
		r.Client,
		r.Scheme,
		r.PlatformLoader,
	)
	r.reconciler.SetRecorder(r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Transform{}).
		Named("transform").
		Complete(r)
}

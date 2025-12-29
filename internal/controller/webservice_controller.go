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

// WebServiceReconciler reconciles a WebService object
type WebServiceReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	reconciler *reconcile.Reconciler
}

// +kubebuilder:rbac:groups=platform.platform.example.com,resources=webservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=webservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.platform.example.com,resources=webservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *WebServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling WebService", "name", req.Name, "namespace", req.Namespace)

	// Use the handler-based reconciler
	return r.reconciler.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize components
	loader := platformloader.NewLoader()
	renderer := platformloader.NewRenderer(loader)

	// Create the handler-based reconciler
	// Note: Execution is now handled by ResourceGraph controller
	r.reconciler = reconcile.NewReconciler(
		r.Client,
		r.Scheme,
		renderer,
	)
	r.reconciler.SetRecorder(r.Recorder)

	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.WebService{}).
		Named("webservice").
		Complete(r)
}

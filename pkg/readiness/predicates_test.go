package readiness

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConditionMatchPredicate(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		predicate *ConditionMatchPredicate
		wantReady bool
		wantErr   bool
	}{
		{
			name: "condition matches",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			},
			predicate: &ConditionMatchPredicate{
				ConditionType:   "Ready",
				ConditionStatus: "True",
			},
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "condition type matches but status doesn't",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{
								"type":   "Ready",
								"status": "False",
							},
						},
					},
				},
			},
			predicate: &ConditionMatchPredicate{
				ConditionType:   "Ready",
				ConditionStatus: "True",
			},
			wantReady: false,
			wantErr:   false,
		},
		{
			name: "no conditions",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]interface{}{
						"name": "test-pod",
					},
				},
			},
			predicate: &ConditionMatchPredicate{
				ConditionType:   "Ready",
				ConditionStatus: "True",
			},
			wantReady: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := fake.NewClientBuilder().Build()

			ready, err := tt.predicate.Evaluate(ctx, c, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ready != tt.wantReady {
				t.Errorf("Evaluate() ready = %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

func TestDeploymentAvailablePredicate(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		wantReady bool
		wantErr   bool
	}{
		{
			name: "deployment available",
			obj: func() *unstructured.Unstructured {
				deployment := &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-deployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}
				unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
				return &unstructured.Unstructured{Object: unstructuredObj}
			}(),
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "deployment not available",
			obj: func() *unstructured.Unstructured {
				deployment := &appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-deployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: corev1.ConditionFalse,
							},
						},
					},
				}
				unstructuredObj, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
				return &unstructured.Unstructured{Object: unstructuredObj}
			}(),
			wantReady: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			c := fake.NewClientBuilder().Build()
			predicate := &DeploymentAvailablePredicate{}

			ready, err := predicate.Evaluate(ctx, c, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ready != tt.wantReady {
				t.Errorf("Evaluate() ready = %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

func TestExistsPredicate(t *testing.T) {
	ctx := context.Background()

	// Create a fake client with a ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
	}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cm).
		Build()

	predicate := &ExistsPredicate{}

	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		wantReady bool
		wantErr   bool
	}{
		{
			name: "resource exists",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "test-cm",
						"namespace": "default",
					},
				},
			},
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "resource does not exist",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
					"metadata": map[string]interface{}{
						"name":      "non-existent",
						"namespace": "default",
					},
				},
			},
			wantReady: false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, err := predicate.Evaluate(ctx, c, tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if ready != tt.wantReady {
				t.Errorf("Evaluate() ready = %v, want %v", ready, tt.wantReady)
			}
		})
	}
}

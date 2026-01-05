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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

const (
	timeout  = time.Second * 60
	interval = time.Millisecond * 250
)

var _ = Describe("ResourceGraph Controller", Ordered, func() {
	Context("When reconciling a ResourceGraph", func() {
		const namespace = "default"
		ctx := context.Background()

		// Helper to delete and wait for resource to be gone
		deleteAndWait := func(obj client.Object, key types.NamespacedName) {
			err := k8sClient.Get(ctx, key, obj)
			if err != nil {
				return // Already gone
			}

			// Delete with background propagation to let finalizers run
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed())

			// Wait for the resource to be fully deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, key, obj)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}

		It("should successfully reconcile a simple ResourceGraph", func() {
			// Use unique names for this test
			resourceGraphName := fmt.Sprintf("test-rg-simple-%d", GinkgoRandomSeed())
			deploymentName := fmt.Sprintf("test-deploy-simple-%d", GinkgoRandomSeed())

			By("Creating a ResourceGraph with a simple Deployment")

			// Create a simple deployment object
			deployment := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}

			// Marshal deployment to JSON
			deploymentJSON, err := json.Marshal(deployment)
			Expect(err).NotTo(HaveOccurred())

			// Create ResourceGraph
			resourceGraph := &platformv1alpha1.ResourceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceGraphName,
					Namespace: namespace,
				},
				Spec: platformv1alpha1.ResourceGraphSpec{
					SourceRef: platformv1alpha1.ObjectReference{
						APIVersion: "pequod.io/v1alpha1",
						Kind:       "Transform",
						Name:       "test-transform",
						Namespace:  namespace,
					},
					Metadata: platformv1alpha1.GraphMetadata{
						Name:    "test-graph",
						Version: "v1alpha1",
					},
					Nodes: []platformv1alpha1.ResourceNode{
						{
							ID:     "deployment",
							Object: runtime.RawExtension{Raw: deploymentJSON},
							ApplyPolicy: platformv1alpha1.ApplyPolicy{
								Mode:           "Apply",
								ConflictPolicy: "Error",
							},
							ReadyWhen: []platformv1alpha1.ReadinessPredicate{
								{
									Type: "Exists",
								},
							},
						},
					},
					RenderHash: "test-hash-123",
					RenderedAt: metav1.Now(),
				},
			}

			Expect(k8sClient.Create(ctx, resourceGraph)).To(Succeed())

			// Cleanup deferred to ensure it runs even if test fails
			defer func() {
				By("Cleaning up the ResourceGraph")
				deleteAndWait(&platformv1alpha1.ResourceGraph{}, types.NamespacedName{Name: resourceGraphName, Namespace: namespace})
				By("Cleaning up the Deployment")
				deleteAndWait(&appsv1.Deployment{}, types.NamespacedName{Name: deploymentName, Namespace: namespace})
			}()

			By("Waiting for the ResourceGraph to be reconciled automatically")
			// The controller will reconcile automatically, no need to manually trigger

			By("Checking that the ResourceGraph status was updated")
			Eventually(func() bool {
				rg := &platformv1alpha1.ResourceGraph{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: resourceGraphName, Namespace: namespace}, rg)
				if err != nil {
					return false
				}
				// Check that phase is set
				return rg.Status.Phase != ""
			}, timeout, interval).Should(BeTrue())

			By("Checking that the Deployment was created")
			Eventually(func() bool {
				dep := &appsv1.Deployment{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, dep)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should handle ResourceGraph with multiple nodes", func() {
			// Use unique names for this test
			resourceGraphName := fmt.Sprintf("test-rg-multi-%d", GinkgoRandomSeed())
			deploymentName := fmt.Sprintf("test-deploy-multi-%d", GinkgoRandomSeed())
			serviceName := fmt.Sprintf("test-svc-multi-%d", GinkgoRandomSeed())
			appLabel := fmt.Sprintf("test-multi-%d", GinkgoRandomSeed())

			By("Creating a ResourceGraph with Deployment and Service")

			// Create deployment
			deployment := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      deploymentName,
					Namespace: namespace,
					Labels:    map[string]string{"app": appLabel},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": appLabel},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": appLabel},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}

			deploymentJSON, err := json.Marshal(deployment)
			Expect(err).NotTo(HaveOccurred())

			// Create service
			service := &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: namespace,
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": appLabel},
					Ports: []corev1.ServicePort{
						{
							Port: 80,
						},
					},
				},
			}

			serviceJSON, err := json.Marshal(service)
			Expect(err).NotTo(HaveOccurred())

			// Create ResourceGraph with both nodes
			resourceGraph := &platformv1alpha1.ResourceGraph{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceGraphName,
					Namespace: namespace,
				},
				Spec: platformv1alpha1.ResourceGraphSpec{
					SourceRef: platformv1alpha1.ObjectReference{
						APIVersion: "pequod.io/v1alpha1",
						Kind:       "Transform",
						Name:       "test-transform-multi",
						Namespace:  namespace,
					},
					Metadata: platformv1alpha1.GraphMetadata{
						Name:    "test-graph-multi",
						Version: "v1alpha1",
					},
					Nodes: []platformv1alpha1.ResourceNode{
						{
							ID:     "deployment",
							Object: runtime.RawExtension{Raw: deploymentJSON},
							ApplyPolicy: platformv1alpha1.ApplyPolicy{
								Mode:           "Apply",
								ConflictPolicy: "Error",
							},
						},
						{
							ID:     "service",
							Object: runtime.RawExtension{Raw: serviceJSON},
							ApplyPolicy: platformv1alpha1.ApplyPolicy{
								Mode:           "Apply",
								ConflictPolicy: "Error",
							},
							DependsOn: []string{"deployment"},
						},
					},
					RenderHash: "test-hash-456",
					RenderedAt: metav1.Now(),
				},
			}

			Expect(k8sClient.Create(ctx, resourceGraph)).To(Succeed())

			// Cleanup deferred to ensure it runs even if test fails
			defer func() {
				By("Cleaning up the ResourceGraph")
				deleteAndWait(&platformv1alpha1.ResourceGraph{}, types.NamespacedName{Name: resourceGraphName, Namespace: namespace})
				By("Cleaning up the Deployment")
				deleteAndWait(&appsv1.Deployment{}, types.NamespacedName{Name: deploymentName, Namespace: namespace})
				By("Cleaning up the Service")
				deleteAndWait(&corev1.Service{}, types.NamespacedName{Name: serviceName, Namespace: namespace})
			}()

			By("Waiting for the ResourceGraph to be reconciled automatically")
			// The controller will reconcile automatically

			By("Checking that both resources were created")
			Eventually(func() bool {
				dep := &appsv1.Deployment{}
				svc := &corev1.Service{}
				depErr := k8sClient.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: namespace}, dep)
				svcErr := k8sClient.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: namespace}, svc)
				return depErr == nil && svcErr == nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})

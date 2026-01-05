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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

var _ = Describe("PlatformInstance Controller", Ordered, func() {
	Context("When reconciling a platform instance", func() {
		const (
			transformName = "instance-test-transform"
			instanceName  = "my-webservice"
			namespace     = "default"
		)

		ctx := context.Background()

		transformNN := types.NamespacedName{
			Name:      transformName,
			Namespace: namespace,
		}

		var generatedCRDName string
		var generatedGVK schema.GroupVersionKind

		BeforeAll(func() {
			By("Creating a Transform that generates a CRD")
			tf := &platformv1alpha1.Transform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformName,
					Namespace: namespace,
				},
				Spec: platformv1alpha1.TransformSpec{
					CueRef: platformv1alpha1.CueReference{
						Type: platformv1alpha1.CueRefTypeEmbedded,
						Ref:  "webservice",
					},
					Group:   "test.pequod.io",
					Version: "v1alpha1",
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the Transform to generate a CRD")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, transformNN, tf)
				if err != nil {
					return false
				}
				return tf.Status.GeneratedCRD != nil && tf.Status.Phase == platformv1alpha1.TransformPhaseReady
			}, timeout, interval).Should(BeTrue())

			generatedCRDName = tf.Status.GeneratedCRD.Name
			generatedGVK = schema.GroupVersionKind{
				Group:   "test.pequod.io",
				Version: "v1alpha1",
				Kind:    tf.Status.GeneratedCRD.Kind,
			}

			By("Verifying the CRD exists")
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: generatedCRDName}, crd)).To(Succeed())

			By("Waiting for CRD to be established")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: generatedCRDName}, crd)
				if err != nil {
					return false
				}
				for _, cond := range crd.Status.Conditions {
					if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})

		AfterAll(func() {
			By("Cleaning up the Transform")
			tf := &platformv1alpha1.Transform{}
			if err := k8sClient.Get(ctx, transformNN, tf); err == nil {
				Expect(k8sClient.Delete(ctx, tf)).To(Succeed())

				// Wait for Transform to be fully deleted
				Eventually(func() bool {
					err := k8sClient.Get(ctx, transformNN, tf)
					return client.IgnoreNotFound(err) == nil && err != nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		AfterEach(func() {
			By("Cleaning up platform instances")
			instance := &unstructured.Unstructured{}
			instance.SetGroupVersionKind(generatedGVK)
			instance.SetName(instanceName)
			instance.SetNamespace(namespace)
			_ = k8sClient.Delete(ctx, instance)

			// Clean up any ResourceGraphs
			rgList := &platformv1alpha1.ResourceGraphList{}
			if err := k8sClient.List(ctx, rgList, client.InNamespace(namespace)); err == nil {
				for _, rg := range rgList.Items {
					if rg.Labels["pequod.io/instance"] == instanceName {
						_ = k8sClient.Delete(ctx, &rg)
					}
				}
			}
		})

		It("should create a ResourceGraph when a platform instance is created", func() {
			By("Creating an instance of the generated CRD")
			instance := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": fmt.Sprintf("%s/%s", generatedGVK.Group, generatedGVK.Version),
					"kind":       generatedGVK.Kind,
					"metadata": map[string]interface{}{
						"name":      instanceName,
						"namespace": namespace,
					},
					"spec": map[string]interface{}{
						"name":     "my-app",
						"replicas": int64(2),
						"image":    "nginx:latest",
						"port":     int64(80),
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			By("Waiting for a ResourceGraph to be created")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				if err := k8sClient.List(ctx, rgList,
					client.InNamespace(namespace),
					client.MatchingLabels{"pequod.io/instance": instanceName}); err != nil {
					return false
				}
				return len(rgList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			By("Verifying the ResourceGraph has correct metadata")
			rgList := &platformv1alpha1.ResourceGraphList{}
			Expect(k8sClient.List(ctx, rgList,
				client.InNamespace(namespace),
				client.MatchingLabels{"pequod.io/instance": instanceName})).To(Succeed())

			Expect(rgList.Items).To(HaveLen(1))
			rg := rgList.Items[0]

			// Check labels
			Expect(rg.Labels).To(HaveKeyWithValue("pequod.io/instance", instanceName))
			Expect(rg.Labels).To(HaveKeyWithValue("pequod.io/transform", transformName))
			Expect(rg.Labels).To(HaveKeyWithValue("pequod.io/instance-kind", generatedGVK.Kind))

			// Check source reference
			Expect(rg.Spec.SourceRef.Kind).To(Equal(generatedGVK.Kind))
			Expect(rg.Spec.SourceRef.Name).To(Equal(instanceName))
			Expect(rg.Spec.SourceRef.Namespace).To(Equal(namespace))

			// Check nodes were rendered
			Expect(len(rg.Spec.Nodes)).To(BeNumerically(">", 0))

			By("Verifying the ResourceGraph contains expected resources")
			// The webservice template should create a Deployment and Service
			var hasDeployment, hasService bool
			for _, node := range rg.Spec.Nodes {
				if node.ID == "deployment" {
					hasDeployment = true
				}
				if node.ID == "service" {
					hasService = true
				}
			}
			Expect(hasDeployment).To(BeTrue(), "ResourceGraph should have a deployment node")
			Expect(hasService).To(BeTrue(), "ResourceGraph should have a service node")
		})

		It("should delete ResourceGraph when platform instance is deleted", func() {
			By("Creating an instance of the generated CRD")
			deleteTestInstanceName := "delete-test-instance"
			instance := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": fmt.Sprintf("%s/%s", generatedGVK.Group, generatedGVK.Version),
					"kind":       generatedGVK.Kind,
					"metadata": map[string]interface{}{
						"name":      deleteTestInstanceName,
						"namespace": namespace,
					},
					"spec": map[string]interface{}{
						"name":     "delete-test-app",
						"replicas": int64(1),
						"image":    "nginx:latest",
						"port":     int64(80),
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			By("Waiting for the ResourceGraph to be created")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				if err := k8sClient.List(ctx, rgList,
					client.InNamespace(namespace),
					client.MatchingLabels{"pequod.io/instance": deleteTestInstanceName}); err != nil {
					return false
				}
				return len(rgList.Items) > 0
			}, timeout, interval).Should(BeTrue())

			By("Deleting the platform instance")
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())

			By("Waiting for the platform instance to be fully deleted")
			Eventually(func() bool {
				latestInstance := &unstructured.Unstructured{}
				latestInstance.SetGroupVersionKind(generatedGVK)
				err := k8sClient.Get(ctx, types.NamespacedName{Name: deleteTestInstanceName, Namespace: namespace}, latestInstance)
				return err != nil // Should be NotFound
			}, timeout, interval).Should(BeTrue())

			By("Verifying no ResourceGraphs remain for the deleted instance")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				if err := k8sClient.List(ctx, rgList,
					client.InNamespace(namespace),
					client.MatchingLabels{"pequod.io/instance": deleteTestInstanceName}); err != nil {
					return false
				}
				return len(rgList.Items) == 0
			}, timeout, interval).Should(BeTrue())
		})
	})
})

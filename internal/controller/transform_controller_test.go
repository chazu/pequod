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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

var _ = Describe("Transform Controller", func() {
	Context("When reconciling a Transform", func() {
		const (
			transformName = "test-transform"
			namespace     = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      transformName,
			Namespace: namespace,
		}

		AfterEach(func() {
			// Cleanup Transform
			tf := &platformv1alpha1.Transform{}
			err := k8sClient.Get(ctx, typeNamespacedName, tf)
			if err == nil {
				By("Cleaning up the Transform")
				Expect(k8sClient.Delete(ctx, tf)).To(Succeed())
			}

			// Cleanup any ResourceGraphs
			rgList := &platformv1alpha1.ResourceGraphList{}
			err = k8sClient.List(ctx, rgList, client.InNamespace(namespace))
			if err == nil {
				for _, rg := range rgList.Items {
					_ = k8sClient.Delete(ctx, &rg)
				}
			}
		})

		It("should create a ResourceGraph for Transform", func() {
			By("Creating a Transform")

			// Create input as RawExtension
			// Note: Use explicit type to avoid JSON float conversion
			type webServiceInput struct {
				Image    string `json:"image"`
				Port     int    `json:"port"`
				Replicas int    `json:"replicas"`
			}
			inputData := webServiceInput{
				Image:    "nginx:latest",
				Port:     80,
				Replicas: 2,
			}
			inputJSON, err := json.Marshal(inputData)
			Expect(err).NotTo(HaveOccurred())

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
					Input: runtime.RawExtension{Raw: inputJSON},
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the ResourceGraph to be created")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				err := k8sClient.List(ctx, rgList, client.InNamespace(namespace))
				if err != nil {
					return false
				}
				// Check if any ResourceGraph was created for this Transform
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == transformName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Checking that the Transform status was updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, tf)
				if err != nil {
					return false
				}
				return tf.Status.ResourceGraphRef != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the ResourceGraph has correct content")
			rgList := &platformv1alpha1.ResourceGraphList{}
			Expect(k8sClient.List(ctx, rgList, client.InNamespace(namespace))).To(Succeed())

			var rg *platformv1alpha1.ResourceGraph
			for i := range rgList.Items {
				if rgList.Items[i].Spec.SourceRef.Name == transformName {
					rg = &rgList.Items[i]
					break
				}
			}

			Expect(rg).NotTo(BeNil())
			Expect(rg.Spec.Nodes).NotTo(BeEmpty())
			Expect(rg.Spec.SourceRef.Kind).To(Equal("Transform"))
			Expect(rg.Spec.SourceRef.Name).To(Equal(transformName))

			// Check owner reference
			Expect(rg.OwnerReferences).To(HaveLen(1))
			Expect(rg.OwnerReferences[0].Kind).To(Equal("Transform"))
			Expect(rg.OwnerReferences[0].Name).To(Equal(transformName))

			// Check labels
			Expect(rg.Labels["pequod.io/transform"]).To(Equal(transformName))
			Expect(rg.Labels["pequod.io/transform-type"]).To(Equal("webservice"))
		})

		It("should update ResourceGraph when Transform input changes", func() {
			By("Creating a Transform")

			// Create input as RawExtension
			// Note: Use explicit type to avoid JSON float conversion
			type webServiceInput struct {
				Image    string `json:"image"`
				Port     int    `json:"port"`
				Replicas int    `json:"replicas"`
			}
			inputData := webServiceInput{
				Image:    "nginx:1.0",
				Port:     80,
				Replicas: 1,
			}
			inputJSON, err := json.Marshal(inputData)
			Expect(err).NotTo(HaveOccurred())

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
					Input: runtime.RawExtension{Raw: inputJSON},
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the initial ResourceGraph")
			var initialHash string
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				err := k8sClient.List(ctx, rgList, client.InNamespace(namespace))
				if err != nil {
					return false
				}
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == transformName {
						initialHash = rg.Spec.RenderHash
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Updating the Transform input")
			updatedInput := webServiceInput{
				Image:    "nginx:2.0",
				Port:     8080,
				Replicas: 3,
			}
			updatedJSON, err := json.Marshal(updatedInput)
			Expect(err).NotTo(HaveOccurred())

			// Retry the update to handle potential conflicts from controller status updates
			Eventually(func() error {
				// Re-fetch the latest version
				latestTf := &platformv1alpha1.Transform{}
				if err := k8sClient.Get(ctx, typeNamespacedName, latestTf); err != nil {
					return err
				}
				latestTf.Spec.Input = runtime.RawExtension{Raw: updatedJSON}
				return k8sClient.Update(ctx, latestTf)
			}, timeout, interval).Should(Succeed())

			By("Waiting for ResourceGraph to be updated with new hash")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				err := k8sClient.List(ctx, rgList, client.InNamespace(namespace))
				if err != nil {
					return false
				}
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == transformName && rg.Spec.RenderHash != initialHash {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})

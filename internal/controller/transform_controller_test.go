/*
Copyright 2024.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

var _ = Describe("Transform Controller", func() {
	Context("When reconciling a WebService with transform label", func() {
		const (
			webserviceName = "test-webservice-transform"
			namespace      = "default"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      webserviceName,
			Namespace: namespace,
		}

		AfterEach(func() {
			// Cleanup WebService
			ws := &platformv1alpha1.WebService{}
			err := k8sClient.Get(ctx, typeNamespacedName, ws)
			if err == nil {
				By("Cleaning up the WebService")
				Expect(k8sClient.Delete(ctx, ws)).To(Succeed())
			}

			// Cleanup any ResourceGraphs
			rgList := &platformv1alpha1.ResourceGraphList{}
			err = k8sClient.List(ctx, rgList, client.InNamespace(namespace))
			if err == nil {
				for _, rg := range rgList.Items {
					Expect(k8sClient.Delete(ctx, &rg)).To(Succeed())
				}
			}
		})

		It("should create a ResourceGraph for WebService with transform label", func() {
			By("Creating a WebService with transform label")
			ws := &platformv1alpha1.WebService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      webserviceName,
					Namespace: namespace,
					Labels: map[string]string{
						"pequod.io/transform": "true",
					},
				},
				Spec: platformv1alpha1.WebServiceSpec{
					Image:    "nginx:latest",
					Port:     80,
					Replicas: ptr(int32(2)),
				},
			}

			Expect(k8sClient.Create(ctx, ws)).To(Succeed())

			By("Waiting for the ResourceGraph to be created")
			Eventually(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				err := k8sClient.List(ctx, rgList, client.InNamespace(namespace))
				if err != nil {
					return false
				}
				// Check if any ResourceGraph was created for this WebService
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == webserviceName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Checking that the WebService status was updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, ws)
				if err != nil {
					return false
				}
				return ws.Status.ResourceGraphRef != nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying the ResourceGraph has correct content")
			rgList := &platformv1alpha1.ResourceGraphList{}
			Expect(k8sClient.List(ctx, rgList, client.InNamespace(namespace))).To(Succeed())

			var rg *platformv1alpha1.ResourceGraph
			for i := range rgList.Items {
				if rgList.Items[i].Spec.SourceRef.Name == webserviceName {
					rg = &rgList.Items[i]
					break
				}
			}

			Expect(rg).NotTo(BeNil())
			Expect(rg.Spec.Nodes).NotTo(BeEmpty())
			Expect(rg.Spec.SourceRef.Kind).To(Equal("WebService"))
			Expect(rg.Spec.SourceRef.Name).To(Equal(webserviceName))

			// Check owner reference
			Expect(rg.OwnerReferences).To(HaveLen(1))
			Expect(rg.OwnerReferences[0].Kind).To(Equal("WebService"))
			Expect(rg.OwnerReferences[0].Name).To(Equal(webserviceName))
		})

		It("should NOT process WebService without transform label", func() {
			By("Creating a WebService WITHOUT transform label")
			ws := &platformv1alpha1.WebService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      webserviceName + "-no-label",
					Namespace: namespace,
					// No transform label
				},
				Spec: platformv1alpha1.WebServiceSpec{
					Image: "nginx:latest",
					Port:  80,
				},
			}

			Expect(k8sClient.Create(ctx, ws)).To(Succeed())

			By("Waiting to ensure no ResourceGraph is created")
			Consistently(func() bool {
				rgList := &platformv1alpha1.ResourceGraphList{}
				err := k8sClient.List(ctx, rgList, client.InNamespace(namespace))
				if err != nil {
					return false
				}
				// Check that no ResourceGraph was created for this WebService
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == webserviceName+"-no-label" {
						return true
					}
				}
				return false
			}, time.Second*3, interval).Should(BeFalse())

			// Cleanup
			Expect(k8sClient.Delete(ctx, ws)).To(Succeed())
		})
	})
})


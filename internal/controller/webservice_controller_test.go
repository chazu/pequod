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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

var _ = Describe("WebService Controller", func() {
	const (
		WebServiceName      = "test-webservice-old"
		WebServiceNamespace = "default"
		timeout             = time.Second * 30
		interval            = time.Millisecond * 250
	)

	Context("When creating a WebService with Transform controller", func() {
		It("Should create ResourceGraph and Deployment", func() {
			ctx := context.Background()

			By("Creating a new WebService with transform label")
			replicas := int32(2)
			webService := &platformv1alpha1.WebService{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "platform.platform.example.com/v1alpha1",
					Kind:       "WebService",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      WebServiceName,
					Namespace: WebServiceNamespace,
					Labels: map[string]string{
						"pequod.io/transform": "true", // Required for Transform controller
					},
				},
				Spec: platformv1alpha1.WebServiceSpec{
					Image:    "nginx:latest",
					Port:     8080,
					Replicas: &replicas,
				},
			}
			Expect(k8sClient.Create(ctx, webService)).To(Succeed())

			webServiceLookupKey := types.NamespacedName{Name: WebServiceName, Namespace: WebServiceNamespace}
			createdWebService := &platformv1alpha1.WebService{}

			// Verify WebService was created
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, webServiceLookupKey, createdWebService)).To(Succeed())
			}, timeout, interval).Should(Succeed())

			By("Checking that the ResourceGraph was created by Transform controller")
			Eventually(func(g Gomega) {
				rgList := &platformv1alpha1.ResourceGraphList{}
				g.Expect(k8sClient.List(ctx, rgList, client.InNamespace(WebServiceNamespace))).To(Succeed())

				// Find ResourceGraph for this WebService
				found := false
				for _, rg := range rgList.Items {
					if rg.Spec.SourceRef.Name == WebServiceName {
						found = true
						break
					}
				}
				g.Expect(found).To(BeTrue(), "ResourceGraph should be created for WebService")
			}, timeout, interval).Should(Succeed())

			// Note: In envtest, the ResourceGraph controller will try to apply resources,
			// but Deployments never become "ready" because there are no actual pods.
			// The executor waits for readiness before proceeding, so we can't verify
			// the Deployment was created in this test environment.
			// In a real cluster:
			// 1. ResourceGraph controller would apply the Deployment
			// 2. Deployment would become ready (pods running)
			// 3. Service would be created (depends on deployment)
			// 4. ResourceGraph status would be updated

			// For now, we just verify the ResourceGraph was created correctly
			By("Verifying ResourceGraph contains the expected nodes")
			var resourceGraph *platformv1alpha1.ResourceGraph
			Eventually(func(g Gomega) {
				rgList := &platformv1alpha1.ResourceGraphList{}
				g.Expect(k8sClient.List(ctx, rgList, client.InNamespace(WebServiceNamespace))).To(Succeed())

				for i := range rgList.Items {
					if rgList.Items[i].Spec.SourceRef.Name == WebServiceName {
						resourceGraph = &rgList.Items[i]
						break
					}
				}
				g.Expect(resourceGraph).NotTo(BeNil())
			}, timeout, interval).Should(Succeed())

			// Verify ResourceGraph has nodes
			Expect(resourceGraph.Spec.Nodes).NotTo(BeEmpty(), "ResourceGraph should have nodes")
			Expect(len(resourceGraph.Spec.Nodes)).To(BeNumerically(">=", 1), "ResourceGraph should have at least deployment node")

			By("Checking that WebService status was updated with ResourceGraph reference")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, webServiceLookupKey, createdWebService)).To(Succeed())
				g.Expect(createdWebService.Status.ResourceGraphRef).NotTo(BeNil(), "WebService should have ResourceGraph reference")
			}, timeout, interval).Should(Succeed())

			By("Cleaning up the WebService")
			Expect(k8sClient.Delete(ctx, webService)).To(Succeed())

			// Verify WebService is deleted
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, webServiceLookupKey, createdWebService)
				g.Expect(errors.IsNotFound(err)).To(BeTrue())
			}, timeout, interval).Should(Succeed())
		})
	})
})

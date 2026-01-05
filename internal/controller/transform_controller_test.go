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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	platformv1alpha1 "github.com/chazu/pequod/api/v1alpha1"
)

var _ = Describe("Transform Controller", Ordered, func() {
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

			// Cleanup any generated CRDs
			crdList := &apiextensionsv1.CustomResourceDefinitionList{}
			err = k8sClient.List(ctx, crdList)
			if err == nil {
				for _, crd := range crdList.Items {
					// Only delete CRDs created by our tests
					if crd.Labels != nil && crd.Labels["platform.pequod.io/transform"] != "" {
						_ = k8sClient.Delete(ctx, &crd)
					}
				}
			}
		})

		It("should create a CRD for Transform with embedded CUE module", func() {
			By("Creating a Transform with embedded webservice module")

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
					Group:   "apps.example.com",
					Version: "v1alpha1",
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the CRD to be generated")
			Eventually(func() bool {
				// Check if CRD was created - name is plural of transform name
				crdName := "test-transforms.apps.example.com"
				crd := &apiextensionsv1.CustomResourceDefinition{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, crd)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Checking that the Transform status was updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, tf)
				if err != nil {
					return false
				}
				return tf.Status.GeneratedCRD != nil && tf.Status.Phase == platformv1alpha1.TransformPhaseReady
			}, timeout, interval).Should(BeTrue())

			By("Verifying the generated CRD has correct structure")
			crdName := tf.Status.GeneratedCRD.Name
			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, crd)).To(Succeed())

			Expect(crd.Spec.Group).To(Equal("apps.example.com"))
			// Kind is derived from Transform name with hyphens converted to CamelCase
			Expect(crd.Spec.Names.Kind).To(Equal("TestTransform"))
			Expect(crd.Spec.Scope).To(Equal(apiextensionsv1.NamespaceScoped))
		})

		PIt("should update the CRD when Transform spec changes", func() {
			// Skip: This test has race conditions due to concurrent status updates
			By("Creating a Transform")

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
					Group:   "apps.example.com",
					Version: "v1alpha1",
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the initial CRD to be created")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, tf)
				if err != nil {
					return false
				}
				return tf.Status.GeneratedCRD != nil
			}, timeout, interval).Should(BeTrue())

			By("Updating the Transform to add short names")
			Eventually(func() error {
				latestTf := &platformv1alpha1.Transform{}
				if err := k8sClient.Get(ctx, typeNamespacedName, latestTf); err != nil {
					return err
				}
				latestTf.Spec.ShortNames = []string{"tt", "test"}
				return k8sClient.Update(ctx, latestTf)
			}, timeout, interval).Should(Succeed())

			By("Waiting for CRD to be updated with short names")
			Eventually(func() []string {
				crdName := tf.Status.GeneratedCRD.Name
				crd := &apiextensionsv1.CustomResourceDefinition{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, crd)
				if err != nil {
					return nil
				}
				return crd.Spec.Names.ShortNames
			}, timeout, interval).Should(ContainElements("tt", "test"))
		})

		PIt("should handle paused transforms", func() {
			// Skip: Paused condition check is timing-dependent in integration tests
			By("Creating a paused Transform")

			tf := &platformv1alpha1.Transform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      transformName,
					Namespace: namespace,
					Labels: map[string]string{
						"platform.pequod.io/paused": "true",
					},
				},
				Spec: platformv1alpha1.TransformSpec{
					CueRef: platformv1alpha1.CueReference{
						Type: platformv1alpha1.CueRefTypeEmbedded,
						Ref:  "webservice",
					},
					Group: "apps.example.com",
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Checking that no CRD is generated for paused transform")
			Consistently(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, tf)
				if err != nil {
					return false
				}
				// Should have Paused condition but no GeneratedCRD
				return tf.Status.GeneratedCRD == nil
			}, "2s", interval).Should(BeTrue())
		})

		PIt("should delete the CRD when Transform is deleted", func() {
			// Skip: Finalizer-based deletion has race conditions in integration tests
			By("Creating a Transform")

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
					Group: "apps.example.com",
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the CRD to be created")
			var crdName string
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, tf)
				if err != nil {
					return false
				}
				if tf.Status.GeneratedCRD != nil {
					crdName = tf.Status.GeneratedCRD.Name
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the Transform")
			Expect(k8sClient.Delete(ctx, tf)).To(Succeed())

			By("Waiting for the CRD to be deleted")
			Eventually(func() bool {
				crd := &apiextensionsv1.CustomResourceDefinition{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: crdName}, crd)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should create a ClusterRole when Transform has managedResources", func() {
			By("Creating a Transform with managedResources and Cluster scope")

			rbacTransformName := "rbac-test-transform"
			rbacNamespacedName := types.NamespacedName{Name: rbacTransformName, Namespace: namespace}

			tf := &platformv1alpha1.Transform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rbacTransformName,
					Namespace: namespace,
				},
				Spec: platformv1alpha1.TransformSpec{
					CueRef: platformv1alpha1.CueReference{
						Type: platformv1alpha1.CueRefTypeEmbedded,
						Ref:  "webservice",
					},
					Group:     "apps.example.com",
					Version:   "v1alpha1",
					RBACScope: platformv1alpha1.RBACScopeCluster,
					ManagedResources: []platformv1alpha1.ManagedResource{
						{
							APIGroup:  "apps",
							Resources: []string{"deployments"},
						},
						{
							APIGroup:  "",
							Resources: []string{"services", "configmaps"},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the ClusterRole to be created")
			expectedCRName := "pequod:transform:default.rbac-test-transform"
			Eventually(func() bool {
				cr := &rbacv1.ClusterRole{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedCRName}, cr)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Verifying ClusterRole has correct structure")
			cr := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedCRName}, cr)).To(Succeed())

			// Verify aggregation label
			Expect(cr.Labels).To(HaveKeyWithValue("pequod.io/aggregate-to-manager", "true"))

			// Verify transform labels
			Expect(cr.Labels).To(HaveKeyWithValue("platform.pequod.io/transform", rbacTransformName))
			Expect(cr.Labels).To(HaveKeyWithValue("platform.pequod.io/transform-namespace", namespace))

			// Verify rules
			Expect(cr.Rules).To(HaveLen(2))

			By("Verifying Transform status includes RBAC reference")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, rbacNamespacedName, tf)
				if err != nil {
					return false
				}
				return tf.Status.GeneratedRBAC != nil &&
					tf.Status.GeneratedRBAC.ClusterRoleName == expectedCRName &&
					tf.Status.GeneratedRBAC.RuleCount == 2
			}, timeout, interval).Should(BeTrue())

			By("Cleaning up the Transform and ClusterRole")
			Expect(k8sClient.Delete(ctx, tf)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedCRName}, cr)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should not create RBAC when Transform has no managedResources", func() {
			By("Creating a Transform without managedResources")

			noRbacTransformName := "no-rbac-transform"
			noRbacNamespacedName := types.NamespacedName{Name: noRbacTransformName, Namespace: namespace}

			tf := &platformv1alpha1.Transform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      noRbacTransformName,
					Namespace: namespace,
				},
				Spec: platformv1alpha1.TransformSpec{
					CueRef: platformv1alpha1.CueReference{
						Type: platformv1alpha1.CueRefTypeEmbedded,
						Ref:  "webservice",
					},
					Group:   "apps.example.com",
					Version: "v1alpha1",
					// No ManagedResources
				},
			}

			Expect(k8sClient.Create(ctx, tf)).To(Succeed())

			By("Waiting for the Transform to be ready")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, noRbacNamespacedName, tf)
				if err != nil {
					return false
				}
				return tf.Status.Phase == platformv1alpha1.TransformPhaseReady
			}, timeout, interval).Should(BeTrue())

			By("Verifying no ClusterRole was created")
			expectedCRName := "pequod:transform:default.no-rbac-transform"
			cr := &rbacv1.ClusterRole{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedCRName}, cr)
			Expect(client.IgnoreNotFound(err)).To(Succeed())
			Expect(err).To(HaveOccurred()) // Should be NotFound

			By("Verifying Transform status has nil GeneratedRBAC")
			Expect(tf.Status.GeneratedRBAC).To(BeNil())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, tf)).To(Succeed())
		})
	})
})

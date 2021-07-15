// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var _ = Describe("Gateway Controller", func() {
	Context("Managed Gateway", func() {
		It("Should surface ready status", func() {
			classKey := types.NamespacedName{Name: "test-gateway-" + rand.String(10)}
			gatewayKey := types.NamespacedName{Name: "contour", Namespace: "projectcontour"}

			gatewayClass := &gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      classKey.Name,
					Namespace: classKey.Namespace,
				},
				Spec: gatewayapi_v1alpha1.GatewayClassSpec{Controller: gatewayClassControllerName},
			}

			gateway := &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayKey.Name,
					Namespace: gatewayKey.Namespace,
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					GatewayClassName: classKey.Name,
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: "HTTPRoute",
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			}

			// Create GatewayClass
			Expect(cl.Create(context.Background(), gatewayClass)).Should(Succeed())

			// Create Gateway
			Expect(cl.Create(context.Background(), gateway)).Should(Succeed())

			By("Expecting ready status")
			Eventually(func() bool {
				gw := &gatewayapi_v1alpha1.Gateway{}
				_ = cl.Get(context.Background(), gatewayKey, gw)
				return isGatewayReady(gw)
			}, timeout, interval).Should(BeTrue())

			// Delete Gateway
			By("Expecting Gateway successful deletion")
			Eventually(func() error {
				gw := &gatewayapi_v1alpha1.Gateway{}
				_ = cl.Get(context.Background(), gatewayKey, gw)
				return cl.Delete(context.Background(), gw)
			}, timeout, interval).Should(Succeed())

			By("Expecting Gateway delete to finish")
			Eventually(func() bool {
				gw := &gatewayapi_v1alpha1.Gateway{}
				return errors.IsNotFound(cl.Get(context.Background(), gatewayKey, gw))
			}, timeout, interval).Should(BeTrue())

			// Delete GatewayClass
			By("Expecting GatewayClass successful deletion")
			Eventually(func() error {
				gc := &gatewayapi_v1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), classKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting GatewayClass delete to finish")
			Eventually(func() bool {
				gc := &gatewayapi_v1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), classKey, gc))
			}, timeout, interval).Should(BeTrue())

		})

	})

	Context("Managed Gateway", func() {
		It("Should surface error status if addresses set", func() {
			classKey := types.NamespacedName{Name: "test-gateway-" + rand.String(10)}
			gatewayKey := types.NamespacedName{Name: "contour", Namespace: "projectcontour"}

			gatewayClass := &gatewayapi_v1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      classKey.Name,
					Namespace: classKey.Namespace,
				},
				Spec: gatewayapi_v1alpha1.GatewayClassSpec{Controller: gatewayClassControllerName},
			}

			gateway := &gatewayapi_v1alpha1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayKey.Name,
					Namespace: gatewayKey.Namespace,
				},
				Spec: gatewayapi_v1alpha1.GatewaySpec{
					Addresses: []gatewayapi_v1alpha1.GatewayAddress{{
						Value: "1.2.3.4",
					}},
					GatewayClassName: classKey.Name,
					Listeners: []gatewayapi_v1alpha1.Listener{{
						Port:     80,
						Protocol: gatewayapi_v1alpha1.HTTPProtocolType,
						Routes: gatewayapi_v1alpha1.RouteBindingSelector{
							Kind: "HTTPRoute",
							Namespaces: &gatewayapi_v1alpha1.RouteNamespaces{
								From: routeSelectTypePtr(gatewayapi_v1alpha1.RouteSelectAll),
							},
						},
					}},
				},
			}

			// Create GatewayClass
			Expect(cl.Create(context.Background(), gatewayClass)).Should(Succeed())

			By("Expecting admitted status")
			Eventually(func() bool {
				gc := &gatewayapi_v1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), classKey, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			// Create Gateway
			Expect(cl.Create(context.Background(), gateway)).Should(Succeed())

			By("Expecting ready status")
			Eventually(func() bool {
				gw := &gatewayapi_v1alpha1.Gateway{}
				_ = cl.Get(context.Background(), gatewayKey, gw)
				return isGatewayReady(gw)
			}, timeout, interval).Should(BeFalse())

			// Delete Gateway
			By("Expecting Gateway successful deletion")
			Eventually(func() error {
				gw := &gatewayapi_v1alpha1.Gateway{}
				_ = cl.Get(context.Background(), gatewayKey, gw)
				return cl.Delete(context.Background(), gw)
			}, timeout, interval).Should(Succeed())

			By("Expecting Gateway delete to finish")
			Eventually(func() bool {
				gw := &gatewayapi_v1alpha1.Gateway{}
				return errors.IsNotFound(cl.Get(context.Background(), gatewayKey, gw))
			}, timeout, interval).Should(BeTrue())

			// Delete GatewayClass
			By("Expecting GatewayClass successful deletion")
			Eventually(func() error {
				gc := &gatewayapi_v1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), classKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting GatewayClass delete to finish")
			Eventually(func() bool {
				gc := &gatewayapi_v1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), classKey, gc))
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func routeSelectTypePtr(rst gatewayapi_v1alpha1.RouteSelectType) *gatewayapi_v1alpha1.RouteSelectType {
	return &rst
}

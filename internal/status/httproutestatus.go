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

package status

import (
	"fmt"
	"time"

	"github.com/projectcontour/contour/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const ConditionNotImplemented gatewayapi_v1alpha1.RouteConditionType = "NotImplemented"
const ConditionInvalid gatewayapi_v1alpha1.RouteConditionType = "Invalid"

type RouteReasonType string

const ReasonNotImplemented RouteReasonType = "NotImplemented"
const ReasonPathMatchType RouteReasonType = "PathMatchType"
const ReasonForwardTo RouteReasonType = "ForwardTo"
const ReasonValid RouteReasonType = "Valid"
const ReasonErrorsExist RouteReasonType = "ErrorsExist"

type HTTPRouteUpdate struct {
	Fullname           types.NamespacedName
	Conditions         []metav1.Condition
	ExistingConditions []metav1.Condition
	GatewayRef         types.NamespacedName
	Generation         int64
	TransitionTime     v1.Time
}

// AddCondition returns a metav1.Condition for a given ConditionType.
func (routeUpdate *HTTPRouteUpdate) AddCondition(cond gatewayapi_v1alpha1.RouteConditionType, status metav1.ConditionStatus, reason RouteReasonType, message string) metav1.Condition {
	newDc := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: routeUpdate.Generation,
	}
	routeUpdate.Conditions = append(routeUpdate.Conditions, newDc)
	return newDc
}

// HTTPRouteAccessor returns a HTTPRouteUpdate that allows a client to build up a list of
// v1.Conditions as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the HTTPRouteUpdate does not need
// to know anything the cache internals.
func (c *Cache) HTTPRouteAccessor(route *gatewayapi_v1alpha1.HTTPRoute) (*HTTPRouteUpdate, func()) {
	pu := &HTTPRouteUpdate{
		Fullname:           k8s.NamespacedNameOf(route),
		Conditions:         []metav1.Condition{},
		ExistingConditions: c.getGatewayConditions(route.Status.Gateways),
		GatewayRef:         c.gatewayRef,
		Generation:         route.Generation,
	}

	return pu, func() {
		c.commitHTTPRoute(pu)
	}
}

func (c *Cache) commitHTTPRoute(pu *HTTPRouteUpdate) {
	if len(pu.Conditions) == 0 {
		return
	}
	c.httpRouteUpdates[pu.Fullname] = pu
}

func (routeUpdate *HTTPRouteUpdate) Mutate(obj interface{}) interface{} {
	o, ok := obj.(*gatewayapi_v1alpha1.HTTPRoute)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in HTTPRouteUpdate status mutator",
			obj, routeUpdate.Fullname.Namespace, routeUpdate.Fullname.Name,
		))
	}

	httpRoute := o.DeepCopy()

	var gatewayStatuses []gatewayapi_v1alpha1.RouteGatewayStatus
	var conditionsToWrite []metav1.Condition

	for _, cond := range routeUpdate.Conditions {

		// Look through the newly calculated conditions, if they already exist
		// on the object or if our obervation is stale, then leave them be,
		// otherwise add them to the list to be written back to the API.
		for _, existingCond := range routeUpdate.ExistingConditions {
			if (cond.Status == existingCond.Status &&
				cond.Reason == existingCond.Reason &&
				cond.Message == existingCond.Message &&
				cond.Type == existingCond.Type) ||
				existingCond.ObservedGeneration > cond.ObservedGeneration {

				cond.ObservedGeneration = existingCond.ObservedGeneration
				cond.LastTransitionTime = existingCond.LastTransitionTime
			}
		}

		conditionsToWrite = append(conditionsToWrite, cond)
	}

	gatewayStatuses = append(gatewayStatuses, gatewayapi_v1alpha1.RouteGatewayStatus{
		GatewayRef: gatewayapi_v1alpha1.GatewayReference{
			Name:      routeUpdate.GatewayRef.Name,
			Namespace: routeUpdate.GatewayRef.Namespace,
		},
		Conditions: conditionsToWrite,
	})

	// Now that we have all the conditions, add them back to the object
	// to get written out.
	for _, rgs := range httpRoute.Status.Gateways {
		if rgs.GatewayRef.Name == routeUpdate.GatewayRef.Name && rgs.GatewayRef.Namespace == routeUpdate.GatewayRef.Namespace {
			continue
		} else {
			gatewayStatuses = append(gatewayStatuses, rgs)
		}
	}

	// Set the GatewayStatuses.
	httpRoute.Status.RouteStatus.Gateways = gatewayStatuses

	return httpRoute
}

func (c *Cache) getGatewayConditions(gatewayStatus []gatewayapi_v1alpha1.RouteGatewayStatus) []metav1.Condition {
	for _, gs := range gatewayStatus {
		if c.gatewayRef.Name == gs.GatewayRef.Name &&
			c.gatewayRef.Namespace == gs.GatewayRef.Namespace {
			return gs.Conditions
		}
	}
	return []metav1.Condition{}
}

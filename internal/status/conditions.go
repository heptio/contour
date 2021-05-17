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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const ResourceHTTPRoute = "httproutes"

const ConditionNotImplemented gatewayapi_v1alpha1.RouteConditionType = "NotImplemented"
const ConditionResolvedRefs gatewayapi_v1alpha1.RouteConditionType = "ResolvedRefs"

type RouteReasonType string

const ReasonNotImplemented RouteReasonType = "NotImplemented"
const ReasonPathMatchType RouteReasonType = "PathMatchType"
const ReasonHeaderMatchType RouteReasonType = "HeaderMatchType"
const ReasonHTTPRouteFilterType RouteReasonType = "HTTPRouteFilterType"
const ReasonDegraded RouteReasonType = "Degraded"
const ReasonValid RouteReasonType = "Valid"
const ReasonErrorsExist RouteReasonType = "ErrorsExist"
const ReasonGatewayAllowMismatch RouteReasonType = "GatewayAllowMismatch"
const ReasonRouteConflict RouteReasonType = "RouteConflict"

type ConditionsUpdate struct {
	FullName           types.NamespacedName
	Conditions         map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition
	ExistingConditions map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition
	GatewayRef         types.NamespacedName
	Resource           string
	Generation         int64
	TransitionTime     metav1.Time
}

// AddCondition returns a metav1.Condition for a given ConditionType.
func (routeUpdate *ConditionsUpdate) AddCondition(cond gatewayapi_v1alpha1.RouteConditionType, status metav1.ConditionStatus, reason RouteReasonType, message string) metav1.Condition {

	if c, ok := routeUpdate.Conditions[cond]; ok {
		message = fmt.Sprintf("%s, %s", c.Message, message)
	}

	newDc := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: routeUpdate.Generation,
	}
	routeUpdate.Conditions[cond] = newDc
	return newDc
}

// ConditionsAccessor returns a ConditionsUpdate that allows a client to build up a list of
// metav1.Conditions as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the ConditionsUpdate does not need
// to know anything the cache internals.
func (c *Cache) ConditionsAccessor(nsName types.NamespacedName, generation int64, resource string, gateways []gatewayapi_v1alpha1.RouteGatewayStatus) (*ConditionsUpdate, func()) {
	pu := &ConditionsUpdate{
		FullName:           nsName,
		Conditions:         make(map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition),
		ExistingConditions: c.getGatewayConditions(gateways),
		GatewayRef:         c.gatewayRef,
		Generation:         generation,
		TransitionTime:     metav1.NewTime(time.Now()),
		Resource:           resource,
	}

	return pu, func() {
		c.commitRoute(pu)
	}
}

func (c *Cache) commitRoute(pu *ConditionsUpdate) {
	if len(pu.Conditions) == 0 {
		return
	}
	c.routeUpdates[pu.FullName] = pu
}

func (routeUpdate *ConditionsUpdate) Mutate(obj interface{}) interface{} {
	o, ok := obj.(*gatewayapi_v1alpha1.HTTPRoute)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in ConditionsUpdate status mutator",
			obj, routeUpdate.FullName.Namespace, routeUpdate.FullName.Name,
		))
	}

	route := o.DeepCopy()

	var gatewayStatuses []gatewayapi_v1alpha1.RouteGatewayStatus
	var conditionsToWrite []metav1.Condition

	for _, cond := range routeUpdate.Conditions {

		// set the Condition's observed generation based on
		// the generation of the route we looked at.
		cond.ObservedGeneration = routeUpdate.Generation
		cond.LastTransitionTime = routeUpdate.TransitionTime

		// is there a newer Condition on the route matching
		// this condition's type? If so, our observation is stale,
		// so don't write it, keep the newer one instead.
		var newerConditionExists bool
		for _, existingCond := range routeUpdate.ExistingConditions {
			if existingCond.Type != cond.Type {
				continue
			}

			if existingCond.ObservedGeneration > cond.ObservedGeneration {
				conditionsToWrite = append(conditionsToWrite, existingCond)
				newerConditionExists = true
				break
			}
		}

		// if we didn't find a newer version of the Condition on the
		// route, then write the one we computed.
		if !newerConditionExists {
			conditionsToWrite = append(conditionsToWrite, cond)
		}
	}

	gatewayStatuses = append(gatewayStatuses, gatewayapi_v1alpha1.RouteGatewayStatus{
		GatewayRef: gatewayapi_v1alpha1.RouteStatusGatewayReference{
			Name:      routeUpdate.GatewayRef.Name,
			Namespace: routeUpdate.GatewayRef.Namespace,
		},
		Conditions: conditionsToWrite,
	})

	// Now that we have all the conditions, add them back to the object
	// to get written out.
	for _, rgs := range route.Status.Gateways {
		if rgs.GatewayRef.Name == routeUpdate.GatewayRef.Name && rgs.GatewayRef.Namespace == routeUpdate.GatewayRef.Namespace {
			continue
		} else {
			gatewayStatuses = append(gatewayStatuses, rgs)
		}
	}

	// Set the GatewayStatuses.
	route.Status.RouteStatus.Gateways = gatewayStatuses

	return route
}

func (c *Cache) getGatewayConditions(gatewayStatus []gatewayapi_v1alpha1.RouteGatewayStatus) map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition {
	for _, gs := range gatewayStatus {
		if c.gatewayRef.Name == gs.GatewayRef.Name &&
			c.gatewayRef.Namespace == gs.GatewayRef.Namespace {

			conditions := make(map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition)
			for _, gsCondition := range gs.Conditions {
				if val, ok := conditions[gatewayapi_v1alpha1.RouteConditionType(gsCondition.Type)]; !ok {
					conditions[gatewayapi_v1alpha1.RouteConditionType(gsCondition.Type)] = val
				}
			}
			return conditions
		}
	}
	return map[gatewayapi_v1alpha1.RouteConditionType]metav1.Condition{}
}

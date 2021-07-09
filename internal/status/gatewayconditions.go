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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayapi_v1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const ResourceGateway = "gateways"

type GatewayReasonType string

const ReasonValidGateway = "Valid"
const ReasonInvalidGateway = "Invalid"

const MessageValidGateway = "Valid Gateway"

type GatewayConditionsUpdate struct {
	FullName           types.NamespacedName
	Conditions         map[gatewayapi_v1alpha1.GatewayConditionType]metav1.Condition
	ExistingConditions map[gatewayapi_v1alpha1.GatewayConditionType]metav1.Condition
	GatewayRef         types.NamespacedName
	Resource           string
	Generation         int64
	TransitionTime     metav1.Time
}

// AddCondition returns a metav1.Condition for a given GatewayConditionType.
func (gatewayUpdate *GatewayConditionsUpdate) AddCondition(cond gatewayapi_v1alpha1.GatewayConditionType, status metav1.ConditionStatus, reason GatewayReasonType, message string) metav1.Condition {

	if c, ok := gatewayUpdate.Conditions[cond]; ok {
		message = fmt.Sprintf("%s, %s", c.Message, message)
	}

	newCond := metav1.Condition{
		Reason:             string(reason),
		Status:             status,
		Type:               string(cond),
		Message:            message,
		LastTransitionTime: metav1.NewTime(clock.Now()),
		ObservedGeneration: gatewayUpdate.Generation,
	}
	gatewayUpdate.Conditions[cond] = newCond
	return newCond
}

// GatewayConditionsAccessor returns a GatewayConditionsUpdate that allows a client to build up a list of
// metav1.Conditions as well as a function to commit the change back to the cache when everything
// is done. The commit function pattern is used so that the GatewayConditionsUpdate does not need
// to know anything the cache internals.
func (c *Cache) GatewayConditionsAccessor(nsName types.NamespacedName, generation int64, resource string, gs *gatewayapi_v1alpha1.GatewayStatus) (*GatewayConditionsUpdate, func()) {
	gu := &GatewayConditionsUpdate{
		FullName:           nsName,
		Conditions:         make(map[gatewayapi_v1alpha1.GatewayConditionType]metav1.Condition),
		ExistingConditions: getGatewayConditions(gs),
		GatewayRef:         c.gatewayRef,
		Generation:         generation,
		TransitionTime:     metav1.NewTime(clock.Now()),
		Resource:           resource,
	}

	return gu, func() {
		c.commitGateway(gu)
	}
}

func (c *Cache) commitGateway(gu *GatewayConditionsUpdate) {
	if len(gu.Conditions) == 0 {
		return
	}
	c.gatewayUpdates[gu.FullName] = gu
}

func getGatewayConditions(gs *gatewayapi_v1alpha1.GatewayStatus) map[gatewayapi_v1alpha1.GatewayConditionType]metav1.Condition {
	conditions := make(map[gatewayapi_v1alpha1.GatewayConditionType]metav1.Condition)
	for _, cond := range gs.Conditions {
		if val, ok := conditions[gatewayapi_v1alpha1.GatewayConditionType(cond.Type)]; !ok {
			conditions[gatewayapi_v1alpha1.GatewayConditionType(cond.Type)] = val
		}
	}
	return conditions
}

func (gatewayUpdate *GatewayConditionsUpdate) Mutate(obj interface{}) interface{} {

	var conditionsToWrite []metav1.Condition

	for _, cond := range gatewayUpdate.Conditions {

		// Set the Condition's observed generation based on
		// the generation of the gateway we looked at.
		cond.ObservedGeneration = gatewayUpdate.Generation
		cond.LastTransitionTime = gatewayUpdate.TransitionTime

		// is there a newer Condition on the gateway matching
		// this condition's type? If so, our observation is stale,
		// so don't write it, keep the newer one instead.
		var newerConditionExists bool
		for _, existingCond := range gatewayUpdate.ExistingConditions {
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
		// gateway, then write the one we computed.
		if !newerConditionExists {
			conditionsToWrite = append(conditionsToWrite, cond)
		}
	}

	switch o := obj.(type) {
	case *gatewayapi_v1alpha1.Gateway:
		gw := o.DeepCopy()
		gw.Status = gatewayapi_v1alpha1.GatewayStatus{
			Conditions: conditionsToWrite,
			// TODO: Manage addresses and listeners.
			// xref: https://github.com/projectcontour/contour/issues/3828
		}
		return gw
	default:
		panic(fmt.Sprintf("Unsupported %T object %s/%s in GatewayConditionsUpdate status mutator",
			obj, gatewayUpdate.FullName.Namespace, gatewayUpdate.FullName.Name,
		))
	}

}

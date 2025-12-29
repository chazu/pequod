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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ConditionTypeRendered indicates the graph artifact has been rendered
	ConditionTypeRendered = "Rendered"

	// ConditionTypePolicyPassed indicates policy validation succeeded
	ConditionTypePolicyPassed = "PolicyPassed"

	// ConditionTypeApplying indicates resources are being applied
	ConditionTypeApplying = "Applying"

	// ConditionTypeReady indicates all resources are ready
	ConditionTypeReady = "Ready"
)



// SetCondition sets or updates a condition
func (ws *WebService) SetCondition(conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	for i, cond := range ws.Status.Conditions {
		if cond.Type == conditionType {
			// Update existing condition
			ws.Status.Conditions[i].Status = status
			ws.Status.Conditions[i].Reason = reason
			ws.Status.Conditions[i].Message = message
			ws.Status.Conditions[i].LastTransitionTime = now
			ws.Status.Conditions[i].ObservedGeneration = ws.Generation
			return
		}
	}

	// Add new condition
	ws.Status.Conditions = append(ws.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: ws.Generation,
	})
}

// GetCondition retrieves a condition by type
func (ws *WebService) GetCondition(conditionType string) *metav1.Condition {
	for _, cond := range ws.Status.Conditions {
		if cond.Type == conditionType {
			return &cond
		}
	}
	return nil
}

// FindStatusCondition retrieves a condition by type (alias for GetCondition for pause package compatibility)
func (ws *WebService) FindStatusCondition(conditionType string) *metav1.Condition {
	return ws.GetCondition(conditionType)
}

// RemoveStatusCondition removes a condition by type
func (ws *WebService) RemoveStatusCondition(conditionType string) {
	for i, cond := range ws.Status.Conditions {
		if cond.Type == conditionType {
			ws.Status.Conditions = append(ws.Status.Conditions[:i], ws.Status.Conditions[i+1:]...)
			return
		}
	}
}

// SetStatusCondition sets a condition (for pause package compatibility)
func (ws *WebService) SetStatusCondition(condition metav1.Condition) {
	ws.SetCondition(condition.Type, condition.Status, condition.Reason, condition.Message)
}

// GetStatusConditions returns a pointer to the conditions slice (for pause package compatibility)
func (ws *WebService) GetStatusConditions() *[]metav1.Condition {
	return &ws.Status.Conditions
}

// IsReady returns true if the Ready condition is True
func (ws *WebService) IsReady() bool {
	cond := ws.GetCondition(ConditionTypeReady)
	return cond != nil && cond.Status == metav1.ConditionTrue
}



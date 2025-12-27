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
	// InventoryModeManaged indicates the resource is managed by the operator
	InventoryModeManaged = "Managed"

	// InventoryModeAdopted indicates the resource was adopted from existing resources
	InventoryModeAdopted = "Adopted"

	// InventoryModeOrphaned indicates the resource is no longer in the graph
	InventoryModeOrphaned = "Orphaned"
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

// AddInventoryItem adds or updates an inventory item
func (ws *WebService) AddInventoryItem(item InventoryItem) {
	// Check if item already exists
	for i, existing := range ws.Status.Inventory {
		if existing.NodeID == item.NodeID {
			// Update existing item
			ws.Status.Inventory[i] = item
			return
		}
	}

	// Add new item
	ws.Status.Inventory = append(ws.Status.Inventory, item)
}

// RemoveInventoryItem removes an inventory item by node ID
func (ws *WebService) RemoveInventoryItem(nodeID string) {
	for i, item := range ws.Status.Inventory {
		if item.NodeID == nodeID {
			ws.Status.Inventory = append(ws.Status.Inventory[:i], ws.Status.Inventory[i+1:]...)
			return
		}
	}
}

// GetInventoryItem retrieves an inventory item by node ID
func (ws *WebService) GetInventoryItem(nodeID string) *InventoryItem {
	for _, item := range ws.Status.Inventory {
		if item.NodeID == nodeID {
			return &item
		}
	}
	return nil
}

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

// IsReady returns true if the Ready condition is True
func (ws *WebService) IsReady() bool {
	cond := ws.GetCondition(ConditionTypeReady)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// MarkOrphaned marks all inventory items as orphaned
func (ws *WebService) MarkOrphaned() {
	for i := range ws.Status.Inventory {
		ws.Status.Inventory[i].Mode = InventoryModeOrphaned
	}
}

// GetOrphanedItems returns all orphaned inventory items
func (ws *WebService) GetOrphanedItems() []InventoryItem {
	var orphaned []InventoryItem
	for _, item := range ws.Status.Inventory {
		if item.Mode == InventoryModeOrphaned {
			orphaned = append(orphaned, item)
		}
	}
	return orphaned
}

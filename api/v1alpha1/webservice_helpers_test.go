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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAddInventoryItem(t *testing.T) {
	ws := &WebService{}

	item1 := InventoryItem{
		NodeID:  "node1",
		Kind:    "Deployment",
		Name:    "test-deployment",
		Mode:    InventoryModeManaged,
		Version: "v1",
	}

	// Add new item
	ws.AddInventoryItem(item1)
	if len(ws.Status.Inventory) != 1 {
		t.Errorf("Expected 1 inventory item, got %d", len(ws.Status.Inventory))
	}

	// Update existing item
	item1Updated := item1
	item1Updated.Mode = InventoryModeOrphaned
	ws.AddInventoryItem(item1Updated)
	if len(ws.Status.Inventory) != 1 {
		t.Errorf("Expected 1 inventory item after update, got %d", len(ws.Status.Inventory))
	}
	if ws.Status.Inventory[0].Mode != InventoryModeOrphaned {
		t.Errorf("Expected mode to be updated to Orphaned, got %s", ws.Status.Inventory[0].Mode)
	}
}

func TestRemoveInventoryItem(t *testing.T) {
	ws := &WebService{
		Status: WebServiceStatus{
			Inventory: []InventoryItem{
				{NodeID: "node1", Kind: "Deployment", Name: "test1", Version: "v1"},
				{NodeID: "node2", Kind: "Service", Name: "test2", Version: "v1"},
			},
		},
	}

	ws.RemoveInventoryItem("node1")
	if len(ws.Status.Inventory) != 1 {
		t.Errorf("Expected 1 inventory item after removal, got %d", len(ws.Status.Inventory))
	}
	if ws.Status.Inventory[0].NodeID != "node2" {
		t.Errorf("Expected remaining item to be node2, got %s", ws.Status.Inventory[0].NodeID)
	}
}

func TestGetInventoryItem(t *testing.T) {
	ws := &WebService{
		Status: WebServiceStatus{
			Inventory: []InventoryItem{
				{NodeID: "node1", Kind: "Deployment", Name: "test1", Version: "v1"},
			},
		},
	}

	item := ws.GetInventoryItem("node1")
	if item == nil {
		t.Error("Expected to find inventory item, got nil")
	}
	if item.NodeID != "node1" {
		t.Errorf("Expected NodeID to be node1, got %s", item.NodeID)
	}

	item = ws.GetInventoryItem("non-existent")
	if item != nil {
		t.Error("Expected nil for non-existent item, got item")
	}
}

func TestSetCondition(t *testing.T) {
	ws := &WebService{}

	// Add new condition
	ws.SetCondition(ConditionTypeRendered, metav1.ConditionTrue, "Rendered", "Graph rendered successfully")
	if len(ws.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(ws.Status.Conditions))
	}

	// Update existing condition
	ws.SetCondition(ConditionTypeRendered, metav1.ConditionFalse, "RenderFailed", "Failed to render graph")
	if len(ws.Status.Conditions) != 1 {
		t.Errorf("Expected 1 condition after update, got %d", len(ws.Status.Conditions))
	}
	if ws.Status.Conditions[0].Status != metav1.ConditionFalse {
		t.Errorf("Expected condition status to be False, got %s", ws.Status.Conditions[0].Status)
	}
}

func TestIsReady(t *testing.T) {
	ws := &WebService{}

	// No Ready condition
	if ws.IsReady() {
		t.Error("Expected IsReady to be false when no Ready condition exists")
	}

	// Ready condition is False
	ws.SetCondition(ConditionTypeReady, metav1.ConditionFalse, "NotReady", "Resources not ready")
	if ws.IsReady() {
		t.Error("Expected IsReady to be false when Ready condition is False")
	}

	// Ready condition is True
	ws.SetCondition(ConditionTypeReady, metav1.ConditionTrue, "Ready", "All resources ready")
	if !ws.IsReady() {
		t.Error("Expected IsReady to be true when Ready condition is True")
	}
}

func TestMarkOrphaned(t *testing.T) {
	ws := &WebService{
		Status: WebServiceStatus{
			Inventory: []InventoryItem{
				{NodeID: "node1", Mode: InventoryModeManaged, Kind: "Deployment", Name: "test1", Version: "v1"},
				{NodeID: "node2", Mode: InventoryModeAdopted, Kind: "Service", Name: "test2", Version: "v1"},
			},
		},
	}

	ws.MarkOrphaned()
	for _, item := range ws.Status.Inventory {
		if item.Mode != InventoryModeOrphaned {
			t.Errorf("Expected all items to be Orphaned, got %s for %s", item.Mode, item.NodeID)
		}
	}
}

func TestGetOrphanedItems(t *testing.T) {
	ws := &WebService{
		Status: WebServiceStatus{
			Inventory: []InventoryItem{
				{NodeID: "node1", Mode: InventoryModeManaged, Kind: "Deployment", Name: "test1", Version: "v1"},
				{NodeID: "node2", Mode: InventoryModeOrphaned, Kind: "Service", Name: "test2", Version: "v1"},
				{NodeID: "node3", Mode: InventoryModeOrphaned, Kind: "ConfigMap", Name: "test3", Version: "v1"},
			},
		},
	}

	orphaned := ws.GetOrphanedItems()
	if len(orphaned) != 2 {
		t.Errorf("Expected 2 orphaned items, got %d", len(orphaned))
	}
}

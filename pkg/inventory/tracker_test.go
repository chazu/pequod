package inventory

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func createTestObject(name, namespace string, gvk schema.GroupVersionKind) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: make(map[string]interface{}),
	}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func TestTracker_RecordApplied(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	obj := createTestObject("my-deployment", "default", gvk)

	item := tracker.RecordApplied("node-1", obj)

	if item.ID != "node-1" {
		t.Errorf("Expected ID 'node-1', got %q", item.ID)
	}

	if item.Status != ItemStatusApplied {
		t.Errorf("Expected status Applied, got %v", item.Status)
	}

	if item.GVK != gvk {
		t.Errorf("Expected GVK %v, got %v", gvk, item.GVK)
	}

	if item.Name != "my-deployment" {
		t.Errorf("Expected name 'my-deployment', got %q", item.Name)
	}

	// Should be retrievable
	retrieved, ok := tracker.Get("node-1")
	if !ok {
		t.Error("Expected to find item")
	}

	if retrieved.ID != item.ID {
		t.Error("Retrieved item doesn't match")
	}
}

func TestTracker_RecordAdopted(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	obj := createTestObject("my-service", "default", gvk)

	item := tracker.RecordAdopted("node-2", obj)

	if item.Status != ItemStatusAdopted {
		t.Errorf("Expected status Adopted, got %v", item.Status)
	}
}

func TestTracker_RecordFailed(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	obj := createTestObject("failed-deployment", "default", gvk)

	item := tracker.RecordFailed("node-3", obj)

	if item.Status != ItemStatusFailed {
		t.Errorf("Expected status Failed, got %v", item.Status)
	}
}

func TestTracker_Remove(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	obj := createTestObject("my-deployment", "default", gvk)

	tracker.RecordApplied("node-1", obj)

	if tracker.Size() != 1 {
		t.Errorf("Expected size 1, got %d", tracker.Size())
	}

	tracker.Remove("node-1")

	if tracker.Size() != 0 {
		t.Errorf("Expected size 0 after remove, got %d", tracker.Size())
	}

	_, ok := tracker.Get("node-1")
	if ok {
		t.Error("Item should be removed")
	}
}

func TestTracker_GetByRef(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	obj := createTestObject("my-deployment", "default", gvk)

	tracker.RecordApplied("node-1", obj)

	// Find by ref
	item, ok := tracker.GetByRef(gvk, "default", "my-deployment")
	if !ok {
		t.Error("Expected to find item by ref")
	}

	if item.ID != "node-1" {
		t.Errorf("Expected ID 'node-1', got %q", item.ID)
	}

	// Not found
	_, ok = tracker.GetByRef(gvk, "default", "other-deployment")
	if ok {
		t.Error("Should not find nonexistent item")
	}
}

func TestTracker_GetAll(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))
	tracker.RecordApplied("node-2", createTestObject("deploy-2", "default", gvk))
	tracker.RecordApplied("node-3", createTestObject("deploy-3", "default", gvk))

	items := tracker.GetAll()

	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}

	// Should be sorted by ID
	for i := 1; i < len(items); i++ {
		if items[i].ID < items[i-1].ID {
			t.Error("Items should be sorted by ID")
		}
	}
}

func TestTracker_FindOrphaned(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))
	tracker.RecordApplied("node-2", createTestObject("deploy-2", "default", gvk))
	tracker.RecordApplied("node-3", createTestObject("deploy-3", "default", gvk))

	// Current graph only has node-1 and node-3
	currentNodeIDs := map[string]bool{
		"node-1": true,
		"node-3": true,
	}

	orphaned := tracker.FindOrphaned(currentNodeIDs)

	if len(orphaned) != 1 {
		t.Errorf("Expected 1 orphaned item, got %d", len(orphaned))
	}

	if len(orphaned) > 0 && orphaned[0].ID != "node-2" {
		t.Errorf("Expected node-2 to be orphaned, got %q", orphaned[0].ID)
	}
}

func TestTracker_FindOrphaned_SkipsPruned(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))
	tracker.RecordApplied("node-2", createTestObject("deploy-2", "default", gvk))

	// Mark node-2 as pruned
	tracker.RecordPruned("node-2")

	// Empty current graph
	currentNodeIDs := map[string]bool{}

	orphaned := tracker.FindOrphaned(currentNodeIDs)

	// Only node-1 should be orphaned (node-2 is already pruned)
	if len(orphaned) != 1 {
		t.Errorf("Expected 1 orphaned item, got %d", len(orphaned))
	}
}

func TestTracker_HasDrift(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	obj := createTestObject("my-deployment", "default", gvk)
	obj.Object["spec"] = map[string]interface{}{
		"replicas": int64(3), // Must use int64 for JSON compatibility
	}

	tracker.RecordApplied("node-1", obj)

	// Same object - no drift
	if tracker.HasDrift("node-1", obj) {
		t.Error("Should not detect drift for same object")
	}

	// Modified object - drift
	modifiedObj := obj.DeepCopy()
	modifiedObj.Object["spec"].(map[string]interface{})["replicas"] = int64(5)

	if !tracker.HasDrift("node-1", modifiedObj) {
		t.Error("Should detect drift for modified object")
	}
}

func TestTracker_Serialization(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))
	tracker.RecordApplied("node-2", createTestObject("deploy-2", "default", gvk))

	// Serialize
	data, err := tracker.Serialize()
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize into new tracker
	newTracker := NewTracker()
	if err := newTracker.Deserialize(data); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if newTracker.Size() != 2 {
		t.Errorf("Expected 2 items after deserialize, got %d", newTracker.Size())
	}

	item, ok := newTracker.Get("node-1")
	if !ok {
		t.Error("Expected to find node-1")
	}

	if item.Name != "deploy-1" {
		t.Errorf("Expected name 'deploy-1', got %q", item.Name)
	}
}

func TestTracker_Clear(t *testing.T) {
	tracker := NewTracker()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))
	tracker.RecordApplied("node-2", createTestObject("deploy-2", "default", gvk))

	tracker.Clear()

	if tracker.Size() != 0 {
		t.Errorf("Expected 0 items after clear, got %d", tracker.Size())
	}
}

func TestTracker_Generation(t *testing.T) {
	tracker := NewTracker()

	initialGen := tracker.Generation()

	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	tracker.RecordApplied("node-1", createTestObject("deploy-1", "default", gvk))

	if tracker.Generation() <= initialGen {
		t.Error("Generation should increase after modification")
	}

	gen := tracker.Generation()
	tracker.Remove("node-1")

	if tracker.Generation() <= gen {
		t.Error("Generation should increase after removal")
	}
}

func TestComputeHash(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	obj1 := createTestObject("deploy-1", "default", gvk)
	obj1.Object["spec"] = map[string]interface{}{"replicas": int64(3)}

	obj2 := createTestObject("deploy-1", "default", gvk)
	obj2.Object["spec"] = map[string]interface{}{"replicas": int64(3)}

	obj3 := createTestObject("deploy-1", "default", gvk)
	obj3.Object["spec"] = map[string]interface{}{"replicas": int64(5)}

	hash1 := ComputeHash(obj1)
	hash2 := ComputeHash(obj2)
	hash3 := ComputeHash(obj3)

	// Same content should produce same hash
	if hash1 != hash2 {
		t.Error("Same content should produce same hash")
	}

	// Different content should produce different hash
	if hash1 == hash3 {
		t.Error("Different content should produce different hash")
	}
}

func TestComputeHash_IgnoresMetadataFields(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	obj1 := createTestObject("deploy-1", "default", gvk)
	obj1.Object["spec"] = map[string]interface{}{"replicas": int64(3)}
	obj1.SetResourceVersion("123")
	obj1.SetUID("uid-1")

	obj2 := createTestObject("deploy-1", "default", gvk)
	obj2.Object["spec"] = map[string]interface{}{"replicas": int64(3)}
	obj2.SetResourceVersion("456") // Different resource version
	obj2.SetUID("uid-2")           // Different UID

	hash1 := ComputeHash(obj1)
	hash2 := ComputeHash(obj2)

	// Should produce same hash despite different metadata
	if hash1 != hash2 {
		t.Error("Hash should ignore metadata fields like resourceVersion and UID")
	}
}

func TestComputeHash_NilObject(t *testing.T) {
	hash := ComputeHash(nil)
	if hash != "" {
		t.Errorf("Expected empty hash for nil object, got %q", hash)
	}
}

func TestNewTrackerFromInventory(t *testing.T) {
	inv := &Inventory{
		Items: map[string]InventoryItem{
			"node-1": {
				ID:        "node-1",
				Name:      "deploy-1",
				Namespace: "default",
				Status:    ItemStatusApplied,
			},
		},
	}

	tracker := NewTrackerFromInventory(inv)

	if tracker.Size() != 1 {
		t.Errorf("Expected 1 item, got %d", tracker.Size())
	}

	item, ok := tracker.Get("node-1")
	if !ok {
		t.Error("Expected to find node-1")
	}

	if item.Name != "deploy-1" {
		t.Errorf("Expected name 'deploy-1', got %q", item.Name)
	}
}

func TestNewTrackerFromInventory_NilInventory(t *testing.T) {
	tracker := NewTrackerFromInventory(nil)

	if tracker.Size() != 0 {
		t.Errorf("Expected 0 items for nil inventory, got %d", tracker.Size())
	}
}

func TestInventoryItem_JSONMarshaling(t *testing.T) {
	item := InventoryItem{
		ID:        "node-1",
		GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		Namespace: "default",
		Name:      "deploy-1",
		Hash:      "abc123",
		Status:    ItemStatusApplied,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var unmarshaled InventoryItem
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if unmarshaled.ID != item.ID {
		t.Errorf("ID mismatch: %q != %q", unmarshaled.ID, item.ID)
	}

	if unmarshaled.Status != item.Status {
		t.Errorf("Status mismatch: %v != %v", unmarshaled.Status, item.Status)
	}
}

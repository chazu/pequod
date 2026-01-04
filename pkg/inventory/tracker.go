package inventory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// InventoryItem represents a tracked resource
type InventoryItem struct {
	// ID is the unique identifier for this item (nodeID from the Graph)
	ID string `json:"id"`

	// GVK is the GroupVersionKind of the resource
	GVK schema.GroupVersionKind `json:"gvk"`

	// Namespace of the resource (empty for cluster-scoped)
	Namespace string `json:"namespace,omitempty"`

	// Name of the resource
	Name string `json:"name"`

	// Hash is the content hash of the resource spec (for drift detection)
	Hash string `json:"hash"`

	// Status tracks the current state of the resource
	Status ItemStatus `json:"status"`
}

// ItemStatus represents the status of an inventory item
type ItemStatus string

const (
	// ItemStatusApplied means the resource was successfully applied
	ItemStatusApplied ItemStatus = "Applied"

	// ItemStatusAdopted means the resource was adopted from existing
	ItemStatusAdopted ItemStatus = "Adopted"

	// ItemStatusOrphaned means the resource is no longer in the graph
	ItemStatusOrphaned ItemStatus = "Orphaned"

	// ItemStatusPruned means the resource was deleted
	ItemStatusPruned ItemStatus = "Pruned"

	// ItemStatusFailed means the resource failed to apply
	ItemStatusFailed ItemStatus = "Failed"
)

// Inventory is a collection of tracked resources
type Inventory struct {
	// Items contains all tracked resources keyed by ID
	Items map[string]InventoryItem `json:"items"`
}

// Tracker manages inventory of applied resources
type Tracker struct {
	mu sync.RWMutex

	// inventory is the current inventory state
	inventory *Inventory

	// generation tracks changes to the inventory
	generation int64
}

// NewTracker creates a new inventory tracker
func NewTracker() *Tracker {
	return &Tracker{
		inventory: &Inventory{
			Items: make(map[string]InventoryItem),
		},
	}
}

// NewTrackerFromInventory creates a tracker from an existing inventory
func NewTrackerFromInventory(inv *Inventory) *Tracker {
	if inv == nil {
		inv = &Inventory{
			Items: make(map[string]InventoryItem),
		}
	}
	if inv.Items == nil {
		inv.Items = make(map[string]InventoryItem)
	}
	return &Tracker{
		inventory: inv,
	}
}

// RecordApplied records that a resource was successfully applied
func (t *Tracker) RecordApplied(id string, obj *unstructured.Unstructured) InventoryItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	item := InventoryItem{
		ID:        id,
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Hash:      ComputeHash(obj),
		Status:    ItemStatusApplied,
	}

	t.inventory.Items[id] = item
	t.generation++

	return item
}

// RecordAdopted records that a resource was adopted
func (t *Tracker) RecordAdopted(id string, obj *unstructured.Unstructured) InventoryItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	item := InventoryItem{
		ID:        id,
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Hash:      ComputeHash(obj),
		Status:    ItemStatusAdopted,
	}

	t.inventory.Items[id] = item
	t.generation++

	return item
}

// RecordFailed records that a resource failed to apply
func (t *Tracker) RecordFailed(id string, obj *unstructured.Unstructured) InventoryItem {
	t.mu.Lock()
	defer t.mu.Unlock()

	item := InventoryItem{
		ID:        id,
		GVK:       obj.GroupVersionKind(),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		Hash:      ComputeHash(obj),
		Status:    ItemStatusFailed,
	}

	t.inventory.Items[id] = item
	t.generation++

	return item
}

// RecordPruned records that a resource was pruned
func (t *Tracker) RecordPruned(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if item, ok := t.inventory.Items[id]; ok {
		item.Status = ItemStatusPruned
		t.inventory.Items[id] = item
		t.generation++
	}
}

// Remove removes an item from the inventory
func (t *Tracker) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.inventory.Items, id)
	t.generation++
}

// Get returns an inventory item by ID
func (t *Tracker) Get(id string) (InventoryItem, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	item, ok := t.inventory.Items[id]
	return item, ok
}

// GetByRef returns an inventory item by GVK/namespace/name
func (t *Tracker) GetByRef(gvk schema.GroupVersionKind, namespace, name string) (InventoryItem, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, item := range t.inventory.Items {
		if item.GVK == gvk && item.Namespace == namespace && item.Name == name {
			return item, true
		}
	}
	return InventoryItem{}, false
}

// GetAll returns a copy of all inventory items
func (t *Tracker) GetAll() []InventoryItem {
	t.mu.RLock()
	defer t.mu.RUnlock()

	items := make([]InventoryItem, 0, len(t.inventory.Items))
	for _, item := range t.inventory.Items {
		items = append(items, item)
	}

	// Sort by ID for deterministic ordering
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	return items
}

// GetInventory returns a copy of the inventory
func (t *Tracker) GetInventory() *Inventory {
	t.mu.RLock()
	defer t.mu.RUnlock()

	items := make(map[string]InventoryItem, len(t.inventory.Items))
	for k, v := range t.inventory.Items {
		items[k] = v
	}

	return &Inventory{Items: items}
}

// FindOrphaned identifies resources that are in the inventory but not in the current graph
func (t *Tracker) FindOrphaned(currentNodeIDs map[string]bool) []InventoryItem {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var orphaned []InventoryItem
	for id, item := range t.inventory.Items {
		// Skip already pruned items
		if item.Status == ItemStatusPruned {
			continue
		}

		// If the item is not in the current graph, it's orphaned
		if !currentNodeIDs[id] {
			item.Status = ItemStatusOrphaned
			orphaned = append(orphaned, item)
		}
	}

	// Sort for deterministic ordering
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].ID < orphaned[j].ID
	})

	return orphaned
}

// MarkOrphaned marks items as orphaned without removing them
func (t *Tracker) MarkOrphaned(ids []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, id := range ids {
		if item, ok := t.inventory.Items[id]; ok {
			item.Status = ItemStatusOrphaned
			t.inventory.Items[id] = item
		}
	}
	t.generation++
}

// HasDrift checks if a resource has drifted from its expected state
func (t *Tracker) HasDrift(id string, currentObj *unstructured.Unstructured) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	item, ok := t.inventory.Items[id]
	if !ok {
		return false // Not tracked, can't detect drift
	}

	currentHash := ComputeHash(currentObj)
	return item.Hash != currentHash
}

// UpdateHash updates the hash for an item (after detecting and accepting drift)
func (t *Tracker) UpdateHash(id string, newHash string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if item, ok := t.inventory.Items[id]; ok {
		item.Hash = newHash
		t.inventory.Items[id] = item
		t.generation++
	}
}

// Size returns the number of items in the inventory
func (t *Tracker) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.inventory.Items)
}

// Generation returns the current generation of the inventory
func (t *Tracker) Generation() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.generation
}

// Clear removes all items from the inventory
func (t *Tracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inventory.Items = make(map[string]InventoryItem)
	t.generation++
}

// ComputeHash computes a content hash for an unstructured object
// This is used for drift detection
func ComputeHash(obj *unstructured.Unstructured) string {
	if obj == nil {
		return ""
	}

	// Create a copy and remove fields that change frequently
	// (resourceVersion, generation, managedFields, etc.)
	objCopy := obj.DeepCopy()
	unstructured.RemoveNestedField(objCopy.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(objCopy.Object, "metadata", "generation")
	unstructured.RemoveNestedField(objCopy.Object, "metadata", "uid")
	unstructured.RemoveNestedField(objCopy.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(objCopy.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(objCopy.Object, "status")

	// Marshal and hash
	data, err := json.Marshal(objCopy.Object)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8]) // First 16 hex chars
}

// Serialize serializes the inventory to JSON
func (t *Tracker) Serialize() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return json.Marshal(t.inventory)
}

// Deserialize deserializes the inventory from JSON
func (t *Tracker) Deserialize(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return fmt.Errorf("failed to deserialize inventory: %w", err)
	}

	if inv.Items == nil {
		inv.Items = make(map[string]InventoryItem)
	}

	t.inventory = &inv
	t.generation++
	return nil
}

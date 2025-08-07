package set

import (
	"testing"
)

func TestNewSet(t *testing.T) {
	s := NewSet()

	if s == nil {
		t.Fatal("NewSet() returned nil")
	}

	if s.m == nil {
		t.Fatal("NewSet() did not initialize internal map")
	}
}

func TestSet_Add(t *testing.T) {
	s := NewSet()

	// Test adding single element
	s.Add("test")

	if !s.Contains("test") {
		t.Error("Add() did not add element correctly")
	}

	// Test adding multiple elements
	s.Add("element1")
	s.Add("element2")

	if !s.Contains("element1") {
		t.Error("Add() did not add element1 correctly")
	}

	if !s.Contains("element2") {
		t.Error("Add() did not add element2 correctly")
	}

	// Test adding duplicate element
	s.Add("test")

	if !s.Contains("test") {
		t.Error("Add() should handle duplicates correctly")
	}
}

func TestSet_Contains(t *testing.T) {
	s := NewSet()

	// Test empty set
	if s.Contains("nonexistent") {
		t.Error("Contains() returned true for element in empty set")
	}

	// Test existing element
	s.Add("exists")

	if !s.Contains("exists") {
		t.Error("Contains() returned false for existing element")
	}

	// Test non-existing element
	if s.Contains("nonexistent") {
		t.Error("Contains() returned true for non-existing element")
	}
}

func TestSet_Remove(t *testing.T) {
	s := NewSet()

	// Add elements
	s.Add("keep")
	s.Add("remove")

	// Remove element
	s.Remove("remove")

	if s.Contains("remove") {
		t.Error("Remove() did not remove element correctly")
	}

	if !s.Contains("keep") {
		t.Error("Remove() incorrectly removed other elements")
	}

	// Test removing non-existent element (should not panic)
	s.Remove("nonexistent")

	// Verify set still works
	if !s.Contains("keep") {
		t.Error("Remove() of non-existent element affected existing elements")
	}
}

func TestSet_Iter(t *testing.T) {
	s := NewSet()

	// Test empty set iteration
	elements := make([]string, 0)
	for elem := range s.Iter() {
		elements = append(elements, elem)
	}

	if len(elements) != 0 {
		t.Error("Iter() on empty set should yield no elements")
	}

	// Test set with elements
	testElements := []string{"a", "b", "c"}
	for _, elem := range testElements {
		s.Add(elem)
	}

	receivedElements := make(map[string]bool)
	for elem := range s.Iter() {
		receivedElements[elem] = true
	}

	if len(receivedElements) != len(testElements) {
		t.Errorf("Iter() yielded %d elements, expected %d", len(receivedElements), len(testElements))
	}

	for _, expected := range testElements {
		if !receivedElements[expected] {
			t.Errorf("Iter() did not yield expected element %s", expected)
		}
	}
}

func TestSet_Integration(t *testing.T) {
	s := NewSet()

	// Test complete workflow
	elements := []string{"go", "python", "rust", "javascript"}

	// Add elements
	for _, elem := range elements {
		s.Add(elem)
	}

	// Verify all elements exist
	for _, elem := range elements {
		if !s.Contains(elem) {
			t.Errorf("Integration test: element %s not found after adding", elem)
		}
	}

	// Remove some elements
	s.Remove("python")
	s.Remove("rust")

	// Verify removal
	if s.Contains("python") {
		t.Error("Integration test: python should have been removed")
	}

	if s.Contains("rust") {
		t.Error("Integration test: rust should have been removed")
	}

	// Verify remaining elements
	if !s.Contains("go") {
		t.Error("Integration test: go should still exist")
	}

	if !s.Contains("javascript") {
		t.Error("Integration test: javascript should still exist")
	}

	// Count remaining elements via iteration
	count := 0
	for range s.Iter() {
		count++
	}

	if count != 2 {
		t.Errorf("Integration test: expected 2 remaining elements, got %d", count)
	}
}

func TestSet_ConcurrentAccess(t *testing.T) {
	s := NewSet()

	// Add some initial elements
	s.Add("initial1")
	s.Add("initial2")

	// Test that iteration doesn't block further operations
	done := make(chan bool)

	go func() {
		for item := range s.Iter() {
			// Process each item - required for test concurrency verification
			_ = item // Use the item to satisfy the linter
		}
		done <- true
	}()

	// Add more elements while iteration might be running
	s.Add("concurrent1")
	s.Add("concurrent2")

	// Wait for iteration to complete
	<-done

	// Verify all elements are accessible
	expectedElements := []string{"initial1", "initial2", "concurrent1", "concurrent2"}
	for _, elem := range expectedElements {
		if !s.Contains(elem) {
			t.Errorf("Concurrent test: element %s not found", elem)
		}
	}
}

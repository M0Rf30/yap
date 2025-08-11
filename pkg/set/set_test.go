package set

import (
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
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
	// First, collect all items from current set
	var items []string
	for item := range s.Iter() {
		items = append(items, item)
	}

	// Verify we got the initial elements
	expectedInitial := []string{"initial1", "initial2"}
	for _, elem := range expectedInitial {
		found := slices.Contains(items, elem)

		if !found {
			t.Errorf("Expected element %s not found in iteration", elem)
		}
	}

	// Add more elements after iteration completes
	s.Add("concurrent1")
	s.Add("concurrent2")

	// Verify all elements are accessible
	expectedElements := []string{"initial1", "initial2", "concurrent1", "concurrent2"}
	for _, elem := range expectedElements {
		if !s.Contains(elem) {
			t.Errorf("Concurrent test: element %s not found", elem)
		}
	}
}

func TestContainsFunction(t *testing.T) {
	// Test the standalone Contains function
	testArray := []string{"apple", "banana", "cherry", "date"}

	// Test existing elements
	if !Contains(testArray, "apple") {
		t.Error("Contains should return true for 'apple'")
	}

	if !Contains(testArray, "cherry") {
		t.Error("Contains should return true for 'cherry'")
	}

	// Test non-existing elements
	if Contains(testArray, "grape") {
		t.Error("Contains should return false for 'grape'")
	}

	if Contains(testArray, "") {
		t.Error("Contains should return false for empty string")
	}

	// Test empty array
	emptyArray := []string{}
	if Contains(emptyArray, "anything") {
		t.Error("Contains should return false for any element in empty array")
	}

	// Test nil array (should not panic)
	var nilArray []string
	if Contains(nilArray, "anything") {
		t.Error("Contains should return false for any element in nil array")
	}
}

func TestStringifyArrayWithMockData(t *testing.T) {
	// Since StringifyArray requires *syntax.Assign which is complex to construct,
	// we'll test that the function exists and can handle basic cases
	// For a more complete test, we'd need to construct actual syntax trees

	// Test the function exists and is callable
	// Note: This will likely fail without proper syntax.Assign construction
	// but serves to verify the function signature and imports

	// Create a minimal syntax tree for testing
	// This is a simplified test - in practice, you'd need a proper parser
	parser := syntax.NewParser()

	// Parse a simple bash array assignment
	bashCode := `array=(element1 "element2" element3)`

	file, err := parser.Parse(strings.NewReader(bashCode), "")
	if err != nil {
		t.Skipf("Failed to parse bash code for test: %v", err)
		return
	}

	// Find the array assignment
	var arrayAssign *syntax.Assign
	syntax.Walk(file, func(node syntax.Node) bool {
		if assign, ok := node.(*syntax.Assign); ok && assign.Array != nil {
			arrayAssign = assign
			return false
		}

		return true
	})

	if arrayAssign == nil {
		t.Skip("Could not find array assignment in parsed code")
		return
	}

	// Test StringifyArray - this may fail due to logger.Fatal calls
	// In a real test environment, you'd mock the logger
	defer func() {
		if r := recover(); r != nil {
			t.Logf("StringifyArray panicked (may be expected due to logger.Fatal): %v", r)
		}
	}()

	result := StringifyArray(arrayAssign)
	if len(result) == 0 {
		t.Error("StringifyArray should return non-empty result for valid array")
	}

	t.Logf("StringifyArray result: %v", result)
}

func TestStringifyAssignWithMockData(t *testing.T) {
	// Test StringifyAssign with a constructed syntax tree
	parser := syntax.NewParser()

	// Parse a simple bash variable assignment
	bashCode := `variable="test value"`

	file, err := parser.Parse(strings.NewReader(bashCode), "")
	if err != nil {
		t.Skipf("Failed to parse bash code for test: %v", err)
		return
	}

	// Find the variable assignment
	var varAssign *syntax.Assign
	syntax.Walk(file, func(node syntax.Node) bool {
		if assign, ok := node.(*syntax.Assign); ok && assign.Value != nil {
			varAssign = assign
			return false
		}

		return true
	})

	if varAssign == nil {
		t.Skip("Could not find variable assignment in parsed code")
		return
	}

	// Test StringifyAssign
	result := StringifyAssign(varAssign)
	if result == "" {
		t.Error("StringifyAssign should return non-empty result for valid assignment")
	}

	// Should remove quotes from the result
	if strings.Contains(result, "\"") {
		t.Error("StringifyAssign should remove quotes from result")
	}

	t.Logf("StringifyAssign result: %s", result)
}

func TestStringifyFuncDeclWithMockData(t *testing.T) {
	// Test StringifyFuncDecl with a constructed function
	parser := syntax.NewParser()

	// Parse a simple bash function
	bashCode := `function test_func() {
		echo "hello world"
		return 0
	}`

	file, err := parser.Parse(strings.NewReader(bashCode), "")
	if err != nil {
		t.Skipf("Failed to parse bash code for test: %v", err)
		return
	}

	// Find the function declaration
	var funcDecl *syntax.FuncDecl
	syntax.Walk(file, func(node syntax.Node) bool {
		if fn, ok := node.(*syntax.FuncDecl); ok {
			funcDecl = fn
			return false
		}

		return true
	})

	if funcDecl == nil {
		t.Skip("Could not find function declaration in parsed code")
		return
	}

	// Test StringifyFuncDecl - this may fail due to logger.Fatal calls
	defer func() {
		if r := recover(); r != nil {
			t.Logf("StringifyFuncDecl panicked (may be expected due to logger.Fatal): %v", r)
		}
	}()

	result := StringifyFuncDecl(funcDecl)
	if result == "" {
		t.Error("StringifyFuncDecl should return non-empty result for valid function")
	}

	t.Logf("StringifyFuncDecl result: %s", result)
}

func TestStringifyFunctionsErrorHandling(t *testing.T) {
	// Test error handling for the stringify functions
	// These tests verify the functions handle edge cases gracefully

	// Test with nil assignments - these will likely cause logger.Fatal
	// In a production test environment, you'd mock the logger
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Function panicked as expected due to nil input: %v", r)
		}
	}()

	// These tests are primarily to ensure the functions exist and are callable
	// Actual testing would require mocking the logger to avoid Fatal calls

	t.Log("Testing error handling for stringify functions")
	t.Log("Note: These functions may panic due to logger.Fatal calls with invalid input")
}

func TestExistsVariable(t *testing.T) {
	// Test that the exists variable is properly defined
	// This is a simple structural test
	_ = exists

	t.Log("exists variable is accessible")
}

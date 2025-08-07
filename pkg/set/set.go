// Package set provides generic set data structure implementation.
package set

var exists = struct{}{}

// Set represents a simple set data structure implemented using a map.
type Set struct {
	m map[string]struct{}
}

// NewSet creates a new Set.
//
// It initializes a new Set with an empty map and returns a pointer to it.
// The returned Set is ready to use.
// Returns a pointer to the newly created Set.
func NewSet() *Set {
	s := &Set{
		m: make(map[string]struct{}),
	}

	return s
}

// Add adds a value to the Set.
//
// value: the value to be added.
func (s *Set) Add(value string) {
	s.m[value] = exists
}

// Contains checks if the given value is present in the set.
//
// value: the value to check for.
// bool: true if the value is present, false otherwise.
func (s *Set) Contains(value string) bool {
	_, c := s.m[value]

	return c
}

// Iter returns a channel that iterates over the elements of the set.
//
// It returns a channel of type string.
func (s *Set) Iter() <-chan string {
	iter := make(chan string)

	go func() {
		for key := range s.m {
			iter <- key
		}

		close(iter)
	}()

	return iter
}

// Remove removes the specified value from the set.
//
// value: the value to be removed from the set.
func (s *Set) Remove(value string) {
	delete(s.m, value)
}

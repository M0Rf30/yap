package set

var exists = struct{}{}

type Set struct {
	m map[string]struct{}
}

func (s *Set) Add(value string) {
	s.m[value] = exists
}

func (s *Set) Contains(value string) bool {
	_, c := s.m[value]

	return c
}

func (s *Set) Iter() <-chan any {
	iter := make(chan any)

	go func() {
		for key := range s.m {
			iter <- key
		}

		close(iter)
	}()

	return iter
}

func NewSet() *Set {
	s := &Set{
		m: make(map[string]struct{}),
	}

	return s
}

func (s *Set) Remove(value string) {
	delete(s.m, value)
}

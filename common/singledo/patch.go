package singledo

import "time"

// Forget drops a finished call's result that stale rejects, so the next Do runs
// fn again, and leaves a call in flight alone so callers still share it.
func (s *Single[T]) Forget(stale func(T) bool) {
	s.ForgetAfter(0, stale)
}

// ForgetAfter is Forget for a result at least age old; a younger one stays
// cached without consulting stale.
func (s *Single[T]) ForgetAfter(age time.Duration, stale func(T) bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if s.result != nil && time.Since(s.result.Time) >= age && stale(s.result.Val) {
		s.result = nil
	}
}

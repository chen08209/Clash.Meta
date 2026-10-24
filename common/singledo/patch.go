package singledo

// Forget drops a finished call's result that stale rejects, so the next Do runs
// fn again, and leaves a call in flight alone so callers still share it.
func (s *Single[T]) Forget(stale func(T) bool) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if s.result != nil && stale(s.result.Val) {
		s.result = nil
	}
}

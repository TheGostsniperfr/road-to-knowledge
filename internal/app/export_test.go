package app

// SetApplyHook lets the external tests make a restructure step fail.
func SetApplyHook(s *Service, hook func(step int) error) { s.applyHook = hook }

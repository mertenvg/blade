package coalesce

func Pointer[T any](args ...*T) *T {
	for _, p := range args {
		if p != nil {
			return p
		}
	}
	return nil
}

func pointer[T any](v T) *T {
	return &v
}

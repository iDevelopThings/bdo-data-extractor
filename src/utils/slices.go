package utils

// AppendUnique appends v to s only if it isn't already present, preserving order.
func AppendUnique[T comparable](s []T, v T) []T {
	for _, e := range s {
		if e == v {
			return s
		}
	}

	return append(s, v)
}

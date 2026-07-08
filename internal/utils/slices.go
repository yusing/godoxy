package utils

// Intersect returns a new slice containing the elements that are present in both input slices.
// This provides a more efficient solution than using two nested loops.
func Intersect[T comparable, Slice ~[]T](slice1 Slice, slice2 Slice) Slice {
	var result Slice
	seen := map[T]struct{}{}

	for i := range slice1 {
		seen[slice1[i]] = struct{}{}
	}

	for i := range slice2 {
		if _, ok := seen[slice2[i]]; ok {
			result = append(result, slice2[i])
		}
	}

	return result
}

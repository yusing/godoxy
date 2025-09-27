package utils

import (
	"slices"
	"strings"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestIntersect(t *testing.T) {
	t.Run("strings", func(t *testing.T) {
		t.Run("no intersection", func(t *testing.T) {
			var (
				slice1 = []string{"a", "b", "c"}
				slice2 = []string{"d", "e", "f"}
				want   []string
			)
			result := Intersect(slice1, slice2)
			slices.Sort(result)
			slices.Sort(want)
			expect.Equal(t, result, want)
		})
		t.Run("intersection", func(t *testing.T) {
			var (
				slice1 = []string{"a", "b", "c"}
				slice2 = []string{"b", "c", "d"}
				want   = []string{"b", "c"}
			)
			result := Intersect(slice1, slice2)
			slices.Sort(result)
			slices.Sort(want)
			expect.Equal(t, result, want)
		})
	})
	t.Run("ints", func(t *testing.T) {
		t.Run("no intersection", func(t *testing.T) {
			var (
				slice1 = []int{1, 2, 3}
				slice2 = []int{4, 5, 6}
				want   []int
			)
			result := Intersect(slice1, slice2)
			slices.Sort(result)
			slices.Sort(want)
			expect.Equal(t, result, want)
		})
		t.Run("intersection", func(t *testing.T) {
			var (
				slice1 = []int{1, 2, 3}
				slice2 = []int{2, 3, 4}
				want   = []int{2, 3}
			)
			result := Intersect(slice1, slice2)
			slices.Sort(result)
			slices.Sort(want)
			expect.Equal(t, result, want)
		})
	})
	t.Run("complex", func(t *testing.T) {
		type T struct {
			A string
			B int
		}
		t.Run("no intersection", func(t *testing.T) {
			var (
				slice1 = []T{{"a", 1}, {"b", 2}, {"c", 3}}
				slice2 = []T{{"d", 4}, {"e", 5}, {"f", 6}}
				want   []T
			)
			result := Intersect(slice1, slice2)
			slices.SortFunc(result, func(i T, j T) int {
				return strings.Compare(i.A, j.A)
			})
			slices.SortFunc(want, func(i T, j T) int {
				return strings.Compare(i.A, j.A)
			})
			expect.Equal(t, result, want)
		})
		t.Run("intersection", func(t *testing.T) {
			var (
				slice1 = []T{{"a", 1}, {"b", 2}, {"c", 3}}
				slice2 = []T{{"b", 2}, {"c", 3}, {"d", 4}}
				want   = []T{{"b", 2}, {"c", 3}}
			)
			result := Intersect(slice1, slice2)
			slices.SortFunc(result, func(i T, j T) int {
				return strings.Compare(i.A, j.A)
			})
			slices.SortFunc(want, func(i T, j T) int {
				return strings.Compare(i.A, j.A)
			})
			expect.Equal(t, result, want)
		})
	})
}

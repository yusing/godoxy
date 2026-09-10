package serialization

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Embedding reflect.Type lets the test pause metadata computation without
// changing the production cache or depending on randomly colliding type hashes.
type gatedMetadataType struct {
	reflect.Type
	entered chan struct{}
	release chan struct{}
}

func (t *gatedMetadataType) NumField() int {
	t.entered <- struct{}{}
	<-t.release
	return t.Type.NumField()
}

func TestGetTypeInfoConcurrentInitialization(t *testing.T) {
	const workers = 16
	typ := &gatedMetadataType{
		Type: reflect.TypeFor[struct {
			Value string `json:"value" aliases:"name" validate:"required"`
		}](),
		entered: make(chan struct{}, workers),
		release: make(chan struct{}),
	}
	results := make(chan typeInfo, workers)
	for range workers {
		go func() {
			results <- getTypeInfo(typ)
		}()
	}

	// All callers must be able to compute the same cold key before any
	// publishes. LoadOrCompute serializes them under the same bucket lock,
	// so this fails deterministically with the old implementation.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	computing := 0
waitForComputations:
	for computing < workers {
		select {
		case <-typ.entered:
			computing++
		case <-timer.C:
			break waitForComputations
		}
	}
	// Release even on failure so the old implementation can finish cleanly.
	close(typ.release)
	require.Equal(t, workers, computing, "metadata computation held the cache lock")

	timer.Reset(10 * time.Second)
	var winner typeInfo
	for i := range workers {
		select {
		case info := <-results:
			require.Equal(t, []int{0}, info.keyFieldIndexes[fnv1IgnoreCaseSnake("name")])
			require.True(t, info.hasValidateTag)
			if i == 0 {
				winner = info
			} else {
				require.Equal(t, reflect.ValueOf(winner.keyFieldIndexes).Pointer(),
					reflect.ValueOf(info.keyFieldIndexes).Pointer(), "callers must reuse the published metadata")
				require.Equal(t, reflect.ValueOf(winner.fieldNames).Pointer(),
					reflect.ValueOf(info.fieldNames).Pointer())
			}
		case <-timer.C:
			t.Fatal("concurrent metadata initialization did not finish")
		}
	}
	cached := getTypeInfo(typ)
	require.Equal(t, reflect.ValueOf(winner.keyFieldIndexes).Pointer(),
		reflect.ValueOf(cached.keyFieldIndexes).Pointer())
	require.Empty(t, typ.entered, "cache hit must not recompute metadata")
}

func TestGetTypeInfoEmbeddedMetadata(t *testing.T) {
	type Fields struct {
		Name    string `json:"display_name" aliases:"name,title" validate:"required"`
		Ignored string `deserialize:"-"`
		Hidden  string `json:"-"`
	}
	type Middle struct {
		*Fields
	}
	type Outer struct {
		Middle
		Count int `json:"count"`
	}

	result := make(chan typeInfo, 1)
	go func() {
		result <- getTypeInfo(reflect.TypeFor[Outer]())
	}()
	var info typeInfo
	select {
	case info = <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("embedded metadata initialization did not finish")
	}
	for _, key := range []string{"display_name", "name", "title"} {
		require.Equal(t, []int{0, 0, 0}, info.keyFieldIndexes[fnv1IgnoreCaseSnake(key)])
		require.Contains(t, info.fieldNames, key)
	}
	require.Equal(t, []int{1}, info.keyFieldIndexes[fnv1IgnoreCaseSnake("count")])
	require.Len(t, info.keyFieldIndexes, 4)
	require.Len(t, info.fieldNames, 4)
	require.True(t, info.hasValidateTag)

	var dst Outer
	require.NoError(t, MapUnmarshalValidate(SerializedObject{"title": "example", "count": 3}, &dst))
	require.NotNil(t, dst.Fields)
	require.Equal(t, "example", dst.Name)
	require.Equal(t, 3, dst.Count)
	require.Error(t, MapUnmarshalValidate(SerializedObject{"name": ""}, &dst))

	child := getTypeInfo(reflect.TypeFor[Fields]())
	require.Equal(t, []int{0}, child.keyFieldIndexes[fnv1IgnoreCaseSnake("name")],
		"embedding must not change cached child indexes")
}


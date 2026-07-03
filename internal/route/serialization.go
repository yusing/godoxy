package route

import (
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
)

func init() {
	_ = serialization.MapUnmarshalValidate(map[string]any{}, &Metadata{})
	_ = serialization.MapUnmarshalValidate(map[string]any{}, &types.HTTPConfig{})
}

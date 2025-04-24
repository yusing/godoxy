package expect

import (
	"github.com/bytedance/sonic"
	"github.com/yusing/go-proxy/internal/common"
)

func init() {
	if common.IsTest {
		sonic.ConfigDefault = sonic.Config{
			SortMapKeys: true,
		}.Froze()
	}
}

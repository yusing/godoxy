package accesslog

import "github.com/yusing/go-proxy/internal/utils"

type (
	Format  string
	Filters struct {
		StatusCodes LogFilter[*StatusCodeRange] `json:"status_codes"`
		Method      LogFilter[HTTPMethod]       `json:"method"`
		Host        LogFilter[Host]             `json:"host"`
		Headers     LogFilter[*HTTPHeader]      `json:"headers"` // header exists or header == value
		CIDR        LogFilter[*CIDR]            `json:"cidr"`
	}
	Fields struct {
		Headers FieldConfig `json:"headers"`
		Query   FieldConfig `json:"query"`
		Cookies FieldConfig `json:"cookies"`
	}
	Config struct {
		BufferSize int        `json:"buffer_size"`
		Format     Format     `json:"format" validate:"oneof=common combined json"`
		Path       string     `json:"path" validate:"required"`
		Filters    Filters    `json:"filters"`
		Fields     Fields     `json:"fields"`
		Retention  *Retention `json:"retention"`
	}
)

var (
	FormatCommon   Format = "common"
	FormatCombined Format = "combined"
	FormatJSON     Format = "json"
)

const DefaultBufferSize = 64 * 1024 // 64KB

func DefaultConfig() *Config {
	return &Config{
		BufferSize: DefaultBufferSize,
		Format:     FormatCombined,
		Fields: Fields{
			Headers: FieldConfig{
				Default: FieldModeDrop,
			},
			Query: FieldConfig{
				Default: FieldModeKeep,
			},
			Cookies: FieldConfig{
				Default: FieldModeDrop,
			},
		},
	}
}

func init() {
	utils.RegisterDefaultValueFactory(DefaultConfig)
}

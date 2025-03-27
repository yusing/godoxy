package types

type Config struct {
	Link    string         `json:"link"`
	Mode    Mode           `json:"mode"`
	Weight  Weight         `json:"weight"`
	Options map[string]any `json:"options,omitempty"`
}

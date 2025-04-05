package provider

type Type string

const (
	TypeDocker Type = "docker"
	TypeFile   Type = "file"
	TypeAgent  Type = "agent"
)

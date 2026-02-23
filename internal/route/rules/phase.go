package rules

import "strings"

type PhaseFlag uint8

const (
	PhaseNone PhaseFlag = 0
	PhasePre  PhaseFlag = 1 << (iota - 1)
	PhasePost
)

func (phase PhaseFlag) IsPostRule() bool {
	return phase&PhasePost != 0
}

func (phase PhaseFlag) String() string {
	if phase == PhaseNone {
		return "none"
	}
	var flags []string
	if phase&PhasePre != 0 {
		flags = append(flags, "PhasePre")
	}
	if phase&PhasePost != 0 {
		flags = append(flags, "PhasePost")
	}
	return strings.Join(flags, ",")
}

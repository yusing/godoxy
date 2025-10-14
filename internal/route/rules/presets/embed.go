package rulepresets

import (
	"embed"
	"fmt"
	"reflect"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/serialization"
)

//go:embed *.yml
var fs embed.FS

var rulePresets = make(map[string]rules.Rules)

func GetRulePreset(name string) (rules.Rules, bool) {
	rules, ok := rulePresets[name]
	return rules, ok
}

// init all rule presets
func init() {
	files, err := fs.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		var rules rules.Rules
		content, err := fs.ReadFile(file.Name())
		if err != nil {
			panic(fmt.Errorf("failed to read rule preset %s: %w", file.Name(), err))
		}
		_, err = serialization.ConvertString(string(content), reflect.ValueOf(&rules))
		if err != nil {
			panic(fmt.Errorf("failed to unmarshal rule preset %s: %w", file.Name(), err))
		}
		rulePresets[file.Name()] = rules
		log.Debug().Str("name", file.Name()).Msg("loaded rule preset")
	}
}

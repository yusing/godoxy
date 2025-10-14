package rulepresets

import (
	"embed"
	"reflect"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/serialization"
)

//go:embed *.yml
var fs embed.FS

var rulePresets = make(map[string]rules.Rules)

var once sync.Once

func GetRulePreset(name string) (rules.Rules, bool) {
	once.Do(initPresets)
	rules, ok := rulePresets[name]
	return rules, ok
}

// init all rule presetsl lazily
func initPresets() {
	files, err := fs.ReadDir(".")
	if err != nil {
		log.Error().Err(err).Msg("failed to read rule presets")
		return
	}
	for _, file := range files {
		var rules rules.Rules
		content, err := fs.ReadFile(file.Name())
		if err != nil {
			log.Error().Str("name", file.Name()).Err(err).Msg("failed to read rule preset")
			continue
		}
		_, err = serialization.ConvertString(string(content), reflect.ValueOf(&rules))
		if err != nil {
			log.Error().Str("name", file.Name()).Err(err).Msg("failed to unmarshal rule preset")
			continue
		}
		rulePresets[file.Name()] = rules
		log.Debug().Str("name", file.Name()).Msg("loaded rule preset")
	}
}

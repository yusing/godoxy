package rulepresets

import (
	"embed"
	"reflect"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
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
			gperr.LogError("failed to read rule preset", err)
			continue
		}
		_, err = serialization.ConvertString(string(content), reflect.ValueOf(&rules))
		if err != nil {
			gperr.LogError("failed to unmarshal rule preset", err)
			continue
		}
		rulePresets[file.Name()] = rules
		log.Debug().Str("name", file.Name()).Msg("loaded rule preset")
	}
}

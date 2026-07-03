package routevalidate

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"reflect"

	"github.com/yusing/godoxy/internal/route"
	rulepresets "github.com/yusing/godoxy/internal/route/rules/presets"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

func validateRules(r *route.Route) error {
	if r.RuleFile != "" && len(r.Rules) > 0 {
		return errors.New("`rule_file` and `rules` cannot be used together")
	} else if r.RuleFile != "" {
		src, err := url.Parse(r.RuleFile)
		if err != nil {
			return fmt.Errorf("failed to parse rule file url %q: %w", r.RuleFile, err)
		}
		switch src.Scheme {
		case "embed": // embed://<preset_file_name>
			rules, ok := rulepresets.GetRulePreset(src.Host)
			if !ok {
				return fmt.Errorf("rule preset %q not found", src.Host)
			}
			r.Rules = rules
		case "file", "":
			if !strutils.IsValidFilename(src.Path) {
				return fmt.Errorf("invalid rule file path %q", src.Path)
			}

			content, err := os.ReadFile(src.Path)
			if err != nil {
				return fmt.Errorf("failed to read rule file %q: %w", src.Path, err)
			}

			_, err = serialization.ConvertString(string(content), reflect.ValueOf(&r.Rules))
			if err != nil {
				return fmt.Errorf("failed to unmarshal rule file %q: %w", src.Path, err)
			}
		default:
			return fmt.Errorf("unsupported rule file scheme %q", src.Scheme)
		}
	}
	return nil
}

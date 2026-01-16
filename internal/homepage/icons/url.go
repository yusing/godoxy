package icons

import (
	"fmt"
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

type (
	URL struct {
		Source `json:"source"`

		FullURL *string `json:"value,omitempty"` // only for absolute/relative icons
		Extra   *Extra  `json:"extra,omitempty"` // only for walkxcode/selfhst icons
	}

	Extra struct {
		Key      Key    `json:"key"`
		Ref      string `json:"ref"`
		FileType string `json:"file_type"`
		IsLight  bool   `json:"is_light"`
		IsDark   bool   `json:"is_dark"`
	}

	Source  string
	Variant string
)

const (
	SourceAbsolute  Source = "https://"
	SourceRelative  Source = "@target"
	SourceWalkXCode Source = "@walkxcode"
	SourceSelfhSt   Source = "@selfhst"
)

const (
	VariantNone  Variant = ""
	VariantLight Variant = "light"
	VariantDark  Variant = "dark"
)

var ErrInvalidIconURL = gperr.New("invalid icon url")

func NewURL(source Source, refOrName, format string) *URL {
	switch source {
	case SourceWalkXCode, SourceSelfhSt:
	default:
		panic("invalid icon source")
	}
	isLight, isDark := false, false
	if strings.HasSuffix(refOrName, "-light") {
		isLight = true
		refOrName = strings.TrimSuffix(refOrName, "-light")
	} else if strings.HasSuffix(refOrName, "-dark") {
		isDark = true
		refOrName = strings.TrimSuffix(refOrName, "-dark")
	}
	return &URL{
		Source: source,
		Extra: &Extra{
			Key:      NewKey(source, refOrName),
			FileType: format,
			Ref:      refOrName,
			IsLight:  isLight,
			IsDark:   isDark,
		},
	}
}

func (u *URL) HasIcon() bool {
	return hasIcon(u)
}

func (u *URL) WithVariant(variant Variant) *URL {
	switch u.Source {
	case SourceWalkXCode, SourceSelfhSt:
	default:
		return u // no variant for absolute/relative icons
	}

	var extra *Extra
	if u.Extra != nil {
		extra = &Extra{
			Key:      u.Extra.Key,
			Ref:      u.Extra.Ref,
			FileType: u.Extra.FileType,
			IsLight:  variant == VariantLight,
			IsDark:   variant == VariantDark,
		}
		extra.Ref = strings.TrimSuffix(extra.Ref, "-light")
		extra.Ref = strings.TrimSuffix(extra.Ref, "-dark")
	}
	return &URL{
		Source:  u.Source,
		FullURL: u.FullURL,
		Extra:   extra,
	}
}

// Parse implements strutils.Parser.
func (u *URL) Parse(v string) error {
	return u.parse(v, true)
}

func (u *URL) parse(v string, checkExists bool) error {
	if v == "" {
		return ErrInvalidIconURL
	}
	slashIndex := strings.Index(v, "/")
	if slashIndex == -1 {
		return ErrInvalidIconURL
	}
	beforeSlash := v[:slashIndex]
	switch beforeSlash {
	case "http:", "https:":
		u.FullURL = &v
		u.Source = SourceAbsolute
	case "@target", "": // @target/favicon.ico, /favicon.ico
		url := v[slashIndex:]
		if url == "/" {
			return ErrInvalidIconURL.Withf("%s", "empty path")
		}
		u.FullURL = &url
		u.Source = SourceRelative
	case "@selfhst", "@walkxcode": // selfh.st / walkxcode Icons, @selfhst/<reference>.<format>
		if beforeSlash == "@selfhst" {
			u.Source = SourceSelfhSt
		} else {
			u.Source = SourceWalkXCode
		}
		parts := strings.Split(v[slashIndex+1:], ".")
		if len(parts) != 2 {
			return ErrInvalidIconURL.Withf("expect @%s/<reference>.<format>, e.g. @%s/adguard-home.webp", beforeSlash, beforeSlash)
		}
		reference, format := parts[0], strings.ToLower(parts[1])
		if reference == "" || format == "" {
			return ErrInvalidIconURL
		}
		switch format {
		case "svg", "png", "webp":
		default:
			return ErrInvalidIconURL.Withf("%s", "invalid image format, expect svg/png/webp")
		}
		isLight, isDark := false, false
		if strings.HasSuffix(reference, "-light") {
			isLight = true
			reference = strings.TrimSuffix(reference, "-light")
		} else if strings.HasSuffix(reference, "-dark") {
			isDark = true
			reference = strings.TrimSuffix(reference, "-dark")
		}
		u.Extra = &Extra{
			Key:      NewKey(u.Source, reference),
			FileType: format,
			Ref:      reference,
			IsLight:  isLight,
			IsDark:   isDark,
		}
		if checkExists && !u.HasIcon() {
			return ErrInvalidIconURL.Withf("no such icon %s.%s from %s", reference, format, u.Source)
		}
	default:
		return ErrInvalidIconURL.Subject(v)
	}

	return nil
}

func (u *URL) URL() string {
	if u.FullURL != nil {
		return *u.FullURL
	}
	if u.Extra == nil {
		return ""
	}
	filename := u.Extra.Ref
	if u.Extra.IsLight {
		filename += "-light"
	} else if u.Extra.IsDark {
		filename += "-dark"
	}
	switch u.Source {
	case SourceWalkXCode:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/%s/%s.%s", u.Extra.FileType, filename, u.Extra.FileType)
	case SourceSelfhSt:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/selfhst/icons/%s/%s.%s", u.Extra.FileType, filename, u.Extra.FileType)
	}
	return ""
}

func (u *URL) String() string {
	if u.FullURL != nil {
		return *u.FullURL
	}
	if u.Extra == nil {
		return ""
	}
	var suffix string
	if u.Extra.IsLight {
		suffix = "-light"
	} else if u.Extra.IsDark {
		suffix = "-dark"
	}
	return fmt.Sprintf("%s/%s%s.%s", u.Source, u.Extra.Ref, suffix, u.Extra.FileType)
}

func (u *URL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *URL) UnmarshalText(data []byte) error {
	return u.parse(string(data), false)
}

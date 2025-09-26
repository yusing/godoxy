package homepage

import (
	"fmt"
	"strings"

	"github.com/yusing/godoxy/internal/gperr"
)

type (
	IconURL struct {
		IconSource `json:"source"`

		FullURL *string    `json:"value,omitempty"` // only for absolute/relative icons
		Extra   *IconExtra `json:"extra,omitempty"` // only for walkxcode/selfhst icons
	}

	IconExtra struct {
		Key      IconKey `json:"key"`
		Ref      string  `json:"ref"`
		FileType string  `json:"file_type"`
		IsLight  bool    `json:"is_light"`
		IsDark   bool    `json:"is_dark"`
	}

	IconSource string
)

const (
	IconSourceAbsolute  IconSource = "https://"
	IconSourceRelative  IconSource = "@target"
	IconSourceWalkXCode IconSource = "@walkxcode"
	IconSourceSelfhSt   IconSource = "@selfhst"
)

var ErrInvalidIconURL = gperr.New("invalid icon url")

func NewIconURL(source IconSource, refOrName, format string) *IconURL {
	switch source {
	case IconSourceWalkXCode, IconSourceSelfhSt:
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
	return &IconURL{
		IconSource: source,
		Extra: &IconExtra{
			Key:      NewIconKey(source, refOrName),
			FileType: format,
			Ref:      refOrName,
			IsLight:  isLight,
			IsDark:   isDark,
		},
	}
}

func NewSelfhStIconURL(refOrName, format string) *IconURL {
	return NewIconURL(IconSourceSelfhSt, refOrName, format)
}

func NewWalkXCodeIconURL(name, format string) *IconURL {
	return NewIconURL(IconSourceWalkXCode, name, format)
}

// HasIcon checks if the icon referenced by the IconURL exists in the cache based on its source.
// Returns false if the icon does not exist for IconSourceSelfhSt or IconSourceWalkXCode,
// otherwise returns true.
func (u *IconURL) HasIcon() bool {
	return HasIcon(u)
}

// Parse implements strutils.Parser.
func (u *IconURL) Parse(v string) error {
	return u.parse(v, true)
}

func (u *IconURL) parse(v string, checkExists bool) error {
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
		u.IconSource = IconSourceAbsolute
	case "@target", "": // @target/favicon.ico, /favicon.ico
		url := v[slashIndex:]
		if url == "/" {
			return ErrInvalidIconURL.Withf("%s", "empty path")
		}
		u.FullURL = &url
		u.IconSource = IconSourceRelative
	case "@selfhst", "@walkxcode": // selfh.st / walkxcode Icons, @selfhst/<reference>.<format>
		if beforeSlash == "@selfhst" {
			u.IconSource = IconSourceSelfhSt
		} else {
			u.IconSource = IconSourceWalkXCode
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
		u.Extra = &IconExtra{
			Key:      NewIconKey(u.IconSource, reference),
			FileType: format,
			Ref:      reference,
			IsLight:  isLight,
			IsDark:   isDark,
		}
		if checkExists && !u.HasIcon() {
			return ErrInvalidIconURL.Withf("no such icon %s.%s from %s", reference, format, u.IconSource)
		}
	default:
		return ErrInvalidIconURL.Subject(v)
	}

	return nil
}

func (u *IconURL) URL() string {
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
	switch u.IconSource {
	case IconSourceWalkXCode:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/%s/%s.%s", u.Extra.FileType, filename, u.Extra.FileType)
	case IconSourceSelfhSt:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/selfhst/icons/%s/%s.%s", u.Extra.FileType, filename, u.Extra.FileType)
	}
	return ""
}

func (u *IconURL) String() string {
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
	return fmt.Sprintf("%s/%s%s.%s", u.IconSource, u.Extra.Ref, suffix, u.Extra.FileType)
}

func (u *IconURL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *IconURL) UnmarshalText(data []byte) error {
	return u.parse(string(data), false)
}

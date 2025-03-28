package homepage

import (
	"fmt"
	"strings"

	"github.com/yusing/go-proxy/internal"
	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	IconURL struct {
		Value      string `json:"value"`
		FullValue  string `json:"full_value"`
		IconSource `json:"source"`
		Extra      *IconExtra `json:"extra"`
	}

	IconExtra struct {
		FileType string `json:"file_type"`
		Name     string `json:"name"`
	}

	IconSource int
)

const (
	IconSourceAbsolute IconSource = iota
	IconSourceRelative
	IconSourceWalkXCode
	IconSourceSelfhSt
)

var ErrInvalidIconURL = gperr.New("invalid icon url")

func NewSelfhStIconURL(reference, format string) *IconURL {
	return &IconURL{
		Value:      reference + "." + format,
		FullValue:  fmt.Sprintf("@selfhst/%s.%s", reference, format),
		IconSource: IconSourceSelfhSt,
		Extra: &IconExtra{
			FileType: format,
			Name:     reference,
		},
	}
}

func NewWalkXCodeIconURL(name, format string) *IconURL {
	return &IconURL{
		Value:      name + "." + format,
		FullValue:  fmt.Sprintf("@walkxcode/%s.%s", name, format),
		IconSource: IconSourceWalkXCode,
		Extra: &IconExtra{
			FileType: format,
			Name:     name,
		},
	}
}

// HasIcon checks if the icon referenced by the IconURL exists in the cache based on its source.
// Returns false if the icon does not exist for IconSourceSelfhSt or IconSourceWalkXCode,
// otherwise returns true.
func (u *IconURL) HasIcon() bool {
	if u.IconSource == IconSourceSelfhSt {
		return internal.HasSelfhstIcon(u.Extra.Name, u.Extra.FileType)
	}
	if u.IconSource == IconSourceWalkXCode {
		return internal.HasWalkxCodeIcon(u.Extra.Name, u.Extra.FileType)
	}
	return true
}

// Parse implements strutils.Parser.
func (u *IconURL) Parse(v string) error {
	if v == "" {
		return ErrInvalidIconURL
	}
	slashIndex := strings.Index(v, "/")
	if slashIndex == -1 {
		return ErrInvalidIconURL
	}
	u.FullValue = v
	beforeSlash := v[:slashIndex]
	switch beforeSlash {
	case "http:", "https:":
		u.Value = v
		u.IconSource = IconSourceAbsolute
	case "@target", "": // @target/favicon.ico, /favicon.ico
		u.Value = v[slashIndex:]
		u.IconSource = IconSourceRelative
		if u.Value == "/" {
			return ErrInvalidIconURL.Withf("%s", "empty path")
		}
	case "png", "svg", "webp": // walkxcode Icons
		u.Value = v
		u.IconSource = IconSourceWalkXCode
		u.Extra = &IconExtra{
			FileType: beforeSlash,
			Name:     strings.TrimSuffix(v[slashIndex+1:], "."+beforeSlash),
		}
	case "@selfhst", "@walkxcode": // selfh.st / walkxcode Icons, @selfhst/<reference>.<format>
		u.Value = v[slashIndex+1:]
		if beforeSlash == "@selfhst" {
			u.IconSource = IconSourceSelfhSt
		} else {
			u.IconSource = IconSourceWalkXCode
		}
		parts := strings.Split(u.Value, ".")
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
		u.Extra = &IconExtra{
			FileType: format,
			Name:     reference,
		}
	default:
		return ErrInvalidIconURL.Withf("%s", v)
	}

	if u.Value == "" {
		return ErrInvalidIconURL.Withf("%s", "empty")
	}

	if !u.HasIcon() {
		return ErrInvalidIconURL.Withf("no such icon %s from %s", u.Value, beforeSlash)
	}
	return nil
}

func (u *IconURL) URL() string {
	switch u.IconSource {
	case IconSourceAbsolute:
		return u.Value
	case IconSourceRelative:
		return "/" + u.Value
	case IconSourceWalkXCode:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/walkxcode/dashboard-icons/%s/%s.%s", u.Extra.FileType, u.Extra.Name, u.Extra.FileType)
	case IconSourceSelfhSt:
		return fmt.Sprintf("https://cdn.jsdelivr.net/gh/selfhst/icons/%s/%s.%s", u.Extra.FileType, u.Extra.Name, u.Extra.FileType)
	}
	return ""
}

func (u *IconURL) String() string {
	return u.FullValue
}

func (u *IconURL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *IconURL) UnmarshalText(data []byte) error {
	return u.Parse(string(data))
}

package icons_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/homepage/icons"
	expect "github.com/yusing/goutils/testing"
)

func TestIconURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue *URL
		wantErr   bool
	}{
		{
			name:  "absolute",
			input: "http://example.com/icon.png",
			wantValue: &URL{
				FullURL: new("http://example.com/icon.png"),
				Source:  SourceAbsolute,
			},
		},
		{
			name:  "relative",
			input: "@target/icon.png",
			wantValue: &URL{
				FullURL: new("/icon.png"),
				Source:  SourceRelative,
			},
		},
		{
			name:  "relative2",
			input: "/icon.png",
			wantValue: &URL{
				FullURL: new("/icon.png"),
				Source:  SourceRelative,
			},
		},
		{
			name:    "relative_empty_path",
			input:   "@target/",
			wantErr: true,
		},
		{
			name:    "relative_empty_path2",
			input:   "/",
			wantErr: true,
		},
		{
			name:  "walkxcode",
			input: "@walkxcode/adguard-home.png",
			wantValue: &URL{
				Source: SourceWalkXCode,
				Extra: &Extra{
					Key:      NewKey(SourceWalkXCode, "adguard-home"),
					FileType: "png",
					Ref:      "adguard-home",
				},
			},
		},
		{
			name:  "walkxcode_light",
			input: "@walkxcode/pfsense-light.png",
			wantValue: &URL{
				Source: SourceWalkXCode,
				Extra: &Extra{
					Key:      NewKey(SourceWalkXCode, "pfsense"),
					FileType: "png",
					Ref:      "pfsense",
					IsLight:  true,
				},
			},
		},
		{
			name:    "walkxcode_invalid_format",
			input:   "foo/walkxcode.png",
			wantErr: true,
		},
		{
			name:  "selfh.st_valid",
			input: "@selfhst/adguard-home.webp",
			wantValue: &URL{
				Source: SourceSelfhSt,
				Extra: &Extra{
					Key:      NewKey(SourceSelfhSt, "adguard-home"),
					FileType: "webp",
					Ref:      "adguard-home",
				},
			},
		},
		{
			name:  "selfh.st_light",
			input: "@selfhst/adguard-home-light.png",
			wantValue: &URL{
				Source: SourceSelfhSt,
				Extra: &Extra{
					Key:      NewKey(SourceSelfhSt, "adguard-home"),
					FileType: "png",
					Ref:      "adguard-home",
					IsLight:  true,
				},
			},
		},
		{
			name:  "selfh.st_dark",
			input: "@selfhst/adguard-home-dark.svg",
			wantValue: &URL{
				Source: SourceSelfhSt,
				Extra: &Extra{
					Key:      NewKey(SourceSelfhSt, "adguard-home"),
					FileType: "svg",
					Ref:      "adguard-home",
					IsDark:   true,
				},
			},
		},
		{
			name:    "selfh.st_invalid",
			input:   "@selfhst/foo",
			wantErr: true,
		},
		{
			name:    "selfh.st_invalid_format",
			input:   "@selfhst/foo.bar",
			wantErr: true,
		},
		{
			name:    "invalid",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &URL{}
			err := u.Parse(tc.input)
			if tc.wantErr {
				expect.ErrorIs(t, ErrInvalidIconURL, err)
			} else {
				expect.NoError(t, err)
				expect.Equal(t, u, tc.wantValue)
			}
		})
	}
}

package homepage_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/homepage"
	expect "github.com/yusing/godoxy/internal/utils/testing"
)

func strPtr(s string) *string {
	return &s
}

func TestIconURL(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue *IconURL
		wantErr   bool
	}{
		{
			name:  "absolute",
			input: "http://example.com/icon.png",
			wantValue: &IconURL{
				FullURL:    strPtr("http://example.com/icon.png"),
				IconSource: IconSourceAbsolute,
			},
		},
		{
			name:  "relative",
			input: "@target/icon.png",
			wantValue: &IconURL{
				FullURL:    strPtr("/icon.png"),
				IconSource: IconSourceRelative,
			},
		},
		{
			name:  "relative2",
			input: "/icon.png",
			wantValue: &IconURL{
				FullURL:    strPtr("/icon.png"),
				IconSource: IconSourceRelative,
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
			wantValue: &IconURL{
				IconSource: IconSourceWalkXCode,
				Extra: &IconExtra{
					Key:      NewIconKey(IconSourceWalkXCode, "adguard-home"),
					FileType: "png",
					Ref:      "adguard-home",
				},
			},
		},
		{
			name:  "walkxcode_light",
			input: "@walkxcode/pfsense-light.png",
			wantValue: &IconURL{
				IconSource: IconSourceWalkXCode,
				Extra: &IconExtra{
					Key:      NewIconKey(IconSourceWalkXCode, "pfsense"),
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
			wantValue: &IconURL{
				IconSource: IconSourceSelfhSt,
				Extra: &IconExtra{
					Key:      NewIconKey(IconSourceSelfhSt, "adguard-home"),
					FileType: "webp",
					Ref:      "adguard-home",
				},
			},
		},
		{
			name:  "selfh.st_light",
			input: "@selfhst/adguard-home-light.png",
			wantValue: &IconURL{
				IconSource: IconSourceSelfhSt,
				Extra: &IconExtra{
					Key:      NewIconKey(IconSourceSelfhSt, "adguard-home"),
					FileType: "png",
					Ref:      "adguard-home",
					IsLight:  true,
				},
			},
		},
		{
			name:  "selfh.st_dark",
			input: "@selfhst/adguard-home-dark.svg",
			wantValue: &IconURL{
				IconSource: IconSourceSelfhSt,
				Extra: &IconExtra{
					Key:      NewIconKey(IconSourceSelfhSt, "adguard-home"),
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
			u := &IconURL{}
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

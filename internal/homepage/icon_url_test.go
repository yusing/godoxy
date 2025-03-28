package homepage

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

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
				Value:      "http://example.com/icon.png",
				IconSource: IconSourceAbsolute,
			},
		},
		{
			name:  "relative",
			input: "@target/icon.png",
			wantValue: &IconURL{
				Value:      "/icon.png",
				IconSource: IconSourceRelative,
			},
		},
		{
			name:  "relative2",
			input: "/icon.png",
			wantValue: &IconURL{
				Value:      "/icon.png",
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
			input: "png/adguard-home.png",
			wantValue: &IconURL{
				Value:      "png/adguard-home.png",
				IconSource: IconSourceWalkXCode,
				Extra: &IconExtra{
					FileType: "png",
					Name:     "adguard-home",
				},
			},
		},
		{
			name:  "walkxcode_alt",
			input: "@walkxcode/adguard-home.png",
			wantValue: &IconURL{
				Value:      "adguard-home.png",
				IconSource: IconSourceWalkXCode,
				Extra: &IconExtra{
					FileType: "png",
					Name:     "adguard-home",
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
			input: "@selfhst/adguard-home.png",
			wantValue: &IconURL{
				Value:      "adguard-home.png",
				IconSource: IconSourceSelfhSt,
				Extra: &IconExtra{
					FileType: "png",
					Name:     "adguard-home",
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
				ExpectError(t, ErrInvalidIconURL, err)
			} else {
				tc.wantValue.FullValue = tc.input
				ExpectNoError(t, err)
				ExpectEqual(t, u, tc.wantValue)
			}
		})
	}
}

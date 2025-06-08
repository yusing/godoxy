package nettypes

import (
	urlPkg "net/url"

	"github.com/yusing/go-proxy/internal/utils"
)

type URL struct {
	_ utils.NoCopy
	urlPkg.URL
}

func MustParseURL(url string) *URL {
	u, err := ParseURL(url)
	if err != nil {
		panic(err)
	}
	return u
}

func ParseURL(url string) (*URL, error) {
	u := &URL{}
	return u, u.Parse(url)
}

func NewURL(url *urlPkg.URL) *URL {
	return &URL{URL: *url}
}

func (u *URL) Parse(url string) error {
	uu, err := urlPkg.Parse(url)
	if err != nil {
		return err
	}
	u.URL = *uu
	return nil
}

func (u *URL) String() string {
	if u == nil {
		return "nil"
	}
	return u.URL.String()
}

func (u *URL) MarshalJSON() (text []byte, err error) {
	if u == nil {
		return []byte("null"), nil
	}
	return []byte("\"" + u.URL.String() + "\""), nil
}

func (u *URL) Equals(other *URL) bool {
	return u.String() == other.String()
}

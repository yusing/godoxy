package route

import (
	"errors"
	"strconv"

	"github.com/bytedance/sonic"
	gperr "github.com/yusing/goutils/errs"
)

type Scheme uint8

var ErrInvalidScheme = errors.New("invalid scheme")

const (
	SchemeHTTP Scheme = 1 << iota
	SchemeHTTPS
	SchemeH2C
	SchemeTCP
	SchemeUDP
	SchemeFileServer
	SchemeNone Scheme = 0

	schemeReverseProxy = SchemeHTTP | SchemeHTTPS | SchemeH2C
	schemeStream       = SchemeTCP | SchemeUDP

	schemeStrHTTP       = "http"
	schemeStrHTTPS      = "https"
	schemeStrH2C        = "h2c"
	schemeStrTCP        = "tcp"
	schemeStrUDP        = "udp"
	schemeStrFileServer = "fileserver"
	schemeStrUnknown    = "unknown"
)

func (s Scheme) String() string {
	switch s {
	case SchemeHTTP:
		return schemeStrHTTP
	case SchemeHTTPS:
		return schemeStrHTTPS
	case SchemeH2C:
		return schemeStrH2C
	case SchemeTCP:
		return schemeStrTCP
	case SchemeUDP:
		return schemeStrUDP
	case SchemeFileServer:
		return schemeStrFileServer
	default:
		return schemeStrUnknown
	}
}

func (s Scheme) MarshalJSON() ([]byte, error) {
	return strconv.AppendQuote(nil, s.String()), nil
}

func (s *Scheme) UnmarshalJSON(data []byte) error {
	var v string
	if err := sonic.Unmarshal(data, &v); err != nil {
		return err
	}
	return s.Parse(v)
}

// Parse implements strutils.Parser
func (s *Scheme) Parse(v string) error {
	switch v {
	case schemeStrHTTP:
		*s = SchemeHTTP
	case schemeStrHTTPS:
		*s = SchemeHTTPS
	case schemeStrH2C:
		*s = SchemeH2C
	case schemeStrTCP:
		*s = SchemeTCP
	case schemeStrUDP:
		*s = SchemeUDP
	case schemeStrFileServer:
		*s = SchemeFileServer
	default:
		return gperr.PrependSubject(ErrInvalidScheme, v)
	}
	return nil
}

func (s Scheme) IsReverseProxy() bool { return s&schemeReverseProxy != 0 }
func (s Scheme) IsStream() bool       { return s&schemeStream != 0 }

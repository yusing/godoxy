package route

import (
	"errors"
	"strconv"
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

type Port struct {
	Listening int `json:"listening"`
	Proxy     int `json:"proxy"`
} // @name Port

var (
	ErrInvalidPortSyntax = errors.New("invalid port syntax, expect [listening_port:]target_port")
	ErrPortOutOfRange    = errors.New("port out of range")
)

// Parse implements strutils.Parser.
func (p *Port) Parse(v string) (err error) {
	parts := strings.Split(v, ":")
	switch len(parts) {
	case 1:
		p.Listening = 0
		p.Proxy, err = strconv.Atoi(v)
	case 2:
		var err2 error
		if parts[0] == "" {
			p.Listening = 0
		} else {
			p.Listening, err = strconv.Atoi(parts[0])
		}
		p.Proxy, err2 = strconv.Atoi(parts[1])
		err = gperr.Join(err, err2)
	default:
		return gperr.PrependSubject(ErrInvalidPortSyntax, v)
	}

	if err != nil {
		return err
	}

	if p.Listening < MinPort || p.Listening > MaxPort {
		return gperr.PrependSubject(ErrPortOutOfRange, strconv.Itoa(p.Listening))
	}

	if p.Proxy < MinPort || p.Proxy > MaxPort {
		return gperr.PrependSubject(ErrPortOutOfRange, strconv.Itoa(p.Proxy))
	}

	return nil
}

func (p *Port) String() string {
	if p.Listening == 0 {
		return strconv.Itoa(p.Proxy)
	}
	return strconv.Itoa(p.Listening) + ":" + strconv.Itoa(p.Proxy)
}

const (
	MinPort = 0
	MaxPort = 65535
)

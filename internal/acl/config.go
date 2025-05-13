package acl

import (
	"net"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	"github.com/yusing/go-proxy/internal/maxmind"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type Config struct {
	Default    string                     `json:"default" validate:"omitempty,oneof=allow deny"` // default: allow
	AllowLocal *bool                      `json:"allow_local"`                                   // default: true
	Allow      Matchers                   `json:"allow"`
	Deny       Matchers                   `json:"deny"`
	Log        *accesslog.ACLLoggerConfig `json:"log"`

	config
}

type config struct {
	defaultAllow bool
	allowLocal   bool
	ipCache      *xsync.MapOf[string, *checkCache]
	logAllowed   bool
	logger       *accesslog.AccessLogger
}

type checkCache struct {
	*maxmind.IPInfo
	allow   bool
	created time.Time
}

const cacheTTL = 1 * time.Minute

func (c *checkCache) Expired() bool {
	return c.created.Add(cacheTTL).Before(utils.TimeNow())
}

//TODO: add stats

const (
	ACLAllow = "allow"
	ACLDeny  = "deny"
)

func (c *Config) Validate() gperr.Error {
	switch c.Default {
	case "", ACLAllow:
		c.defaultAllow = true
	case ACLDeny:
		c.defaultAllow = false
	default:
		return gperr.New("invalid default value").Subject(c.Default)
	}

	if c.AllowLocal != nil {
		c.allowLocal = *c.AllowLocal
	} else {
		c.allowLocal = true
	}

	if c.Log != nil {
		c.logAllowed = c.Log.LogAllowed
	}

	c.ipCache = xsync.NewMapOf[string, *checkCache]()
	return nil
}

func (c *Config) Valid() bool {
	return c != nil && (len(c.Allow) > 0 || len(c.Deny) > 0 || c.allowLocal)
}

func (c *Config) Start(parent *task.Task) gperr.Error {
	if c.Log != nil {
		logger, err := accesslog.NewAccessLogger(parent, c.Log)
		if err != nil {
			return gperr.New("failed to start access logger").With(err)
		}
		c.logger = logger
	}
	return nil
}

func (c *Config) cacheRecord(info *maxmind.IPInfo, allow bool) {
	if common.ForceResolveCountry && info.City == nil {
		maxmind.LookupCity(info)
	}
	c.ipCache.Store(info.Str, &checkCache{
		IPInfo:  info,
		allow:   allow,
		created: utils.TimeNow(),
	})
}

func (c *config) log(info *maxmind.IPInfo, allowed bool) {
	if c.logger == nil {
		return
	}
	if !allowed || c.logAllowed {
		c.logger.LogACL(info, !allowed)
	}
}

func (c *Config) IPAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// always allow loopback
	// loopback is not logged
	if ip.IsLoopback() {
		return true
	}

	if c.allowLocal && ip.IsPrivate() {
		c.log(&maxmind.IPInfo{IP: ip, Str: ip.String()}, true)
		return true
	}

	ipStr := ip.String()
	record, ok := c.ipCache.Load(ipStr)
	if ok && !record.Expired() {
		c.log(record.IPInfo, record.allow)
		return record.allow
	}

	ipAndStr := &maxmind.IPInfo{IP: ip, Str: ipStr}
	if c.Allow.Match(ipAndStr) {
		c.log(ipAndStr, true)
		c.cacheRecord(ipAndStr, true)
		return true
	}
	if c.Deny.Match(ipAndStr) {
		c.log(ipAndStr, false)
		c.cacheRecord(ipAndStr, false)
		return false
	}

	c.log(ipAndStr, c.defaultAllow)
	c.cacheRecord(ipAndStr, c.defaultAllow)
	return c.defaultAllow
}

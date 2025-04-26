package acl

import (
	"net"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"github.com/puzpuzpuz/xsync/v3"
	"github.com/rs/zerolog"
	acl "github.com/yusing/go-proxy/internal/acl/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
)

type Config struct {
	Default    string                     `json:"default" validate:"omitempty,oneof=allow deny"` // default: allow
	AllowLocal *bool                      `json:"allow_local"`                                   // default: true
	Allow      []string                   `json:"allow"`
	Deny       []string                   `json:"deny"`
	Log        *accesslog.ACLLoggerConfig `json:"log"`

	MaxMind *MaxMindConfig `json:"maxmind" validate:"omitempty"`

	config
}

type (
	MaxMindDatabaseType string
	MaxMindConfig       struct {
		AccountID  string              `json:"account_id" validate:"required"`
		LicenseKey string              `json:"license_key" validate:"required"`
		Database   MaxMindDatabaseType `json:"database" validate:"required,oneof=geolite geoip2"`

		logger     zerolog.Logger
		lastUpdate time.Time
		db         struct {
			*maxminddb.Reader
			sync.RWMutex
		}
	}
)

type config struct {
	defaultAllow bool
	allowLocal   bool
	allow        []matcher
	deny         []matcher
	ipCache      *xsync.MapOf[string, *checkCache]
	logAllowed   bool
	logger       *accesslog.AccessLogger
}

type checkCache struct {
	*acl.IPInfo
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

const (
	MaxMindGeoLite MaxMindDatabaseType = "geolite"
	MaxMindGeoIP2  MaxMindDatabaseType = "geoip2"
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

	if c.MaxMind != nil {
		c.MaxMind.logger = logging.With().Str("type", string(c.MaxMind.Database)).Logger()
	}

	if c.Log != nil {
		c.logAllowed = c.Log.LogAllowed
	}

	errs := gperr.NewBuilder("syntax error")
	c.allow = make([]matcher, 0, len(c.Allow))
	c.deny = make([]matcher, 0, len(c.Deny))

	for _, s := range c.Allow {
		m, err := c.parseMatcher(s)
		if err != nil {
			errs.Add(err.Subject(s))
			continue
		}
		c.allow = append(c.allow, m)
	}
	for _, s := range c.Deny {
		m, err := c.parseMatcher(s)
		if err != nil {
			errs.Add(err.Subject(s))
			continue
		}
		c.deny = append(c.deny, m)
	}

	if errs.HasError() {
		c.allow = nil
		c.deny = nil
		return errMatcherFormat.With(errs.Error())
	}

	c.ipCache = xsync.NewMapOf[string, *checkCache]()
	return nil
}

func (c *Config) Valid() bool {
	return c != nil && (len(c.allow) > 0 || len(c.deny) > 0 || c.allowLocal)
}

func (c *Config) Start(parent *task.Task) gperr.Error {
	if c.MaxMind != nil {
		if err := c.MaxMind.LoadMaxMindDB(parent); err != nil {
			return err
		}
	}
	if c.Log != nil {
		logger, err := accesslog.NewAccessLogger(parent, c.Log)
		if err != nil {
			return gperr.New("failed to start access logger").With(err)
		}
		c.logger = logger
	}
	return nil
}

func (c *config) cacheRecord(info *acl.IPInfo, allow bool) {
	c.ipCache.Store(info.Str, &checkCache{
		IPInfo:  info,
		allow:   allow,
		created: utils.TimeNow(),
	})
}

func (c *config) log(info *acl.IPInfo, allowed bool) {
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

	// always allow private and loopback
	// loopback is not logged
	if ip.IsLoopback() {
		return true
	}

	if c.allowLocal && ip.IsPrivate() {
		c.log(&acl.IPInfo{IP: ip, Str: ip.String()}, true)
		return true
	}

	ipStr := ip.String()
	record, ok := c.ipCache.Load(ipStr)
	if ok && !record.Expired() {
		c.log(record.IPInfo, record.allow)
		return record.allow
	}

	ipAndStr := &acl.IPInfo{IP: ip, Str: ipStr}
	for _, m := range c.allow {
		if m(ipAndStr) {
			c.log(ipAndStr, true)
			c.cacheRecord(ipAndStr, true)
			return true
		}
	}
	for _, m := range c.deny {
		if m(ipAndStr) {
			c.log(ipAndStr, false)
			c.cacheRecord(ipAndStr, false)
			return false
		}
	}

	c.log(ipAndStr, c.defaultAllow)
	c.cacheRecord(ipAndStr, c.defaultAllow)
	return c.defaultAllow
}

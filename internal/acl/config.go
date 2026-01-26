package acl

import (
	"fmt"
	"math"
	"net"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/maxmind"
	"github.com/yusing/godoxy/internal/notif"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

type Config struct {
	Default    string                     `json:"default" validate:"omitempty,oneof=allow deny"` // default: allow
	AllowLocal *bool                      `json:"allow_local"`                                   // default: true
	Allow      Matchers                   `json:"allow"`
	Deny       Matchers                   `json:"deny"`
	Log        *accesslog.ACLLoggerConfig `json:"log"`

	Notify struct {
		To             []string      `json:"to,omitempty"`             // list of notification providers
		Interval       time.Duration `json:"interval,omitempty"`       // interval between notifications
		IncludeAllowed *bool         `json:"include_allowed,omitzero"` // default: false
	} `json:"notify"`

	config
	valErr gperr.Error
}

const defaultNotifyInterval = 1 * time.Minute

type config struct {
	defaultAllow bool
	allowLocal   bool
	ipCache      *xsync.Map[string, *checkCache]

	// will be nil if Notify.To is empty
	// these are per IP, reset every Notify.Interval
	allowedCount map[string]uint32
	blockedCount map[string]uint32

	// these are total, never reset
	totalAllowedCount uint64
	totalBlockedCount uint64

	logAllowed bool
	// will be nil if Log is nil
	logger accesslog.AccessLogger

	// will never tick if Notify.To is empty
	notifyTicker  *time.Ticker
	notifyAllowed bool

	// will be nil if both Log and Notify.To are empty
	logNotifyCh chan ipLog
}

type checkCache struct {
	*maxmind.IPInfo
	allow   bool
	created time.Time
}

type ipLog struct {
	info    *maxmind.IPInfo
	allowed bool
}

type ContextKey struct{}

const cacheTTL = 1 * time.Minute

func (c *checkCache) Expired() bool {
	return c.created.Add(cacheTTL).Before(time.Now())
}

// TODO: add stats

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
		c.valErr = gperr.New("invalid default value").Subject(c.Default)
		return c.valErr
	}

	if c.AllowLocal != nil {
		c.allowLocal = *c.AllowLocal
	} else {
		c.allowLocal = true
	}

	if c.Notify.Interval <= 0 {
		c.Notify.Interval = defaultNotifyInterval
	}

	if c.Log != nil {
		c.logAllowed = c.Log.LogAllowed
	}

	if !c.allowLocal && !c.defaultAllow && len(c.Allow) == 0 {
		c.valErr = gperr.New("allow_local is false and default is deny, but no allow rules are configured")
		return c.valErr
	}

	c.ipCache = xsync.NewMap[string, *checkCache]()

	if c.Notify.IncludeAllowed != nil {
		c.notifyAllowed = *c.Notify.IncludeAllowed
	} else {
		c.notifyAllowed = false
	}
	return nil
}

func (c *Config) Valid() bool {
	return c != nil && c.valErr == nil
}

func (c *Config) Start(parent task.Parent) gperr.Error {
	if c.Log != nil {
		logger, err := accesslog.NewAccessLogger(parent, c.Log)
		if err != nil {
			return gperr.New("failed to start access logger").With(err)
		}
		c.logger = logger
	}
	if c.valErr != nil {
		return c.valErr
	}

	if c.needLogOrNotify() {
		c.logNotifyCh = make(chan ipLog, 100)
	}

	if c.needNotify() {
		c.allowedCount = make(map[string]uint32)
		c.blockedCount = make(map[string]uint32)
		c.notifyTicker = time.NewTicker(c.Notify.Interval)
	} else {
		c.notifyTicker = time.NewTicker(time.Duration(math.MaxInt64)) // never tick
	}

	if c.needLogOrNotify() {
		go c.logNotifyLoop(parent)
	}

	log.Info().
		Str("default", c.Default).
		Bool("allow_local", c.allowLocal).
		Int("allow_rules", len(c.Allow)).
		Int("deny_rules", len(c.Deny)).
		Msg("ACL started")
	return nil
}

func (c *Config) cacheRecord(info *maxmind.IPInfo, allow bool) {
	if common.ForceResolveCountry && info.City == nil {
		maxmind.LookupCity(info)
	}
	c.ipCache.Store(info.Str, &checkCache{
		IPInfo:  info,
		allow:   allow,
		created: time.Now(),
	})
}

func (c *Config) needLogOrNotify() bool {
	return c.needLog() || c.needNotify()
}

func (c *Config) needLog() bool {
	return c.logger != nil
}

func (c *Config) needNotify() bool {
	return len(c.Notify.To) > 0
}

func (c *Config) getCachedCity(ip string) string {
	record, ok := c.ipCache.Load(ip)
	if ok {
		if record.City != nil {
			if record.City.Country.IsoCode != "" {
				return record.City.Country.IsoCode
			}
			return record.City.Location.TimeZone
		}
	}
	return "unknown location"
}

func (c *Config) logNotifyLoop(parent task.Parent) {
	defer c.notifyTicker.Stop()

	for {
		select {
		case <-parent.Context().Done():
			return
		case log := <-c.logNotifyCh:
			if c.logger != nil {
				if !log.allowed || c.logAllowed {
					c.logger.LogACL(log.info, !log.allowed)
				}
			}
			if c.needNotify() {
				if log.allowed {
					if c.notifyAllowed {
						c.allowedCount[log.info.Str]++
						c.totalAllowedCount++
					}
				} else {
					c.blockedCount[log.info.Str]++
					c.totalBlockedCount++
				}
			}
		case <-c.notifyTicker.C: // will never tick when notify is disabled
			total := len(c.allowedCount) + len(c.blockedCount)
			if total == 0 {
				continue
			}
			total++
			fieldsBody := make(notif.ListBody, total)
			i := 0
			fieldsBody[i] = fmt.Sprintf("Total: allowed %d, blocked %d", c.totalAllowedCount, c.totalBlockedCount)
			i++
			for ip, count := range c.allowedCount {
				fieldsBody[i] = fmt.Sprintf("%s (%s): allowed %d times", ip, c.getCachedCity(ip), count)
				i++
			}
			for ip, count := range c.blockedCount {
				fieldsBody[i] = fmt.Sprintf("%s (%s): blocked %d times", ip, c.getCachedCity(ip), count)
				i++
			}
			notif.Notify(&notif.LogMessage{
				Level: zerolog.InfoLevel,
				Title: "ACL Summary for last " + strutils.FormatDuration(c.Notify.Interval),
				Body:  fieldsBody,
				To:    c.Notify.To,
			})
			clear(c.allowedCount)
			clear(c.blockedCount)
		}
	}
}

// log and notify if needed
func (c *Config) logAndNotify(info *maxmind.IPInfo, allowed bool) {
	if c.logNotifyCh != nil {
		c.logNotifyCh <- ipLog{info: info, allowed: allowed}
	}
}

func (c *Config) IPAllowed(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// always allow loopback, not logged
	if ip.IsLoopback() {
		return true
	}

	if c.allowLocal && ip.IsPrivate() {
		c.logAndNotify(&maxmind.IPInfo{IP: ip, Str: ip.String()}, true)
		return true
	}

	ipStr := ip.String()
	record, ok := c.ipCache.Load(ipStr)
	if ok && !record.Expired() {
		c.logAndNotify(record.IPInfo, record.allow)
		return record.allow
	}

	ipAndStr := &maxmind.IPInfo{IP: ip, Str: ipStr}
	if c.Deny.Match(ipAndStr) {
		c.logAndNotify(ipAndStr, false)
		c.cacheRecord(ipAndStr, false)
		return false
	}
	if c.Allow.Match(ipAndStr) {
		c.logAndNotify(ipAndStr, true)
		c.cacheRecord(ipAndStr, true)
		return true
	}

	c.logAndNotify(ipAndStr, c.defaultAllow)
	c.cacheRecord(ipAndStr, c.defaultAllow)
	return c.defaultAllow
}

package acl

import (
	"fmt"
	"math"
	"net"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/maxmind"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/utils"
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
		To             []string      `json:"to"`              // list of notification providers
		Interval       time.Duration `json:"interval"`        // interval between notifications
		IncludeAllowed *bool         `json:"include_allowed"` // default: false
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
	allowCounts   map[string]uint32
	blockedCounts map[string]uint32

	logAllowed bool
	// will be nil if Log is nil
	logger *accesslog.AccessLogger

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

// could be nil
var ActiveConfig atomic.Pointer[Config]

const cacheTTL = 1 * time.Minute

func (c *checkCache) Expired() bool {
	return c.created.Add(cacheTTL).Before(utils.TimeNow())
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

	if c.Log != nil {
		c.logAllowed = c.Log.LogAllowed
	}

	if !c.allowLocal && !c.defaultAllow && len(c.Allow) == 0 {
		c.valErr = gperr.New("allow_local is false and default is deny, but no allow rules are configured")
		return c.valErr
	}

	c.ipCache = xsync.NewMap[string, *checkCache]()

	if c.needLogOrNotify() {
		c.logNotifyCh = make(chan ipLog, 100)
	}

	if c.needNotify() {
		c.allowCounts = make(map[string]uint32)
		c.blockedCounts = make(map[string]uint32)
	}

	if c.Notify.Interval < 0 {
		c.Notify.Interval = defaultNotifyInterval
	}
	if c.needNotify() {
		c.notifyTicker = time.NewTicker(c.Notify.Interval)
	} else {
		c.notifyTicker = time.NewTicker(time.Duration(math.MaxInt64)) // never tick
	}

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
		created: utils.TimeNow(),
	})
}

func (c *Config) needLogOrNotify() bool {
	return c.logger != nil || c.needNotify()
}

func (c *Config) needNotify() bool {
	return len(c.Notify.To) > 0
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
						c.allowCounts[log.info.Str]++
					}
				} else {
					c.blockedCounts[log.info.Str]++
				}
			}
		case <-c.notifyTicker.C: // will never tick when notify is disabled
			total := len(c.allowCounts) + len(c.blockedCounts)
			if total == 0 {
				continue
			}
			fieldsBody := make(notif.FieldsBody, 0, total)
			for ip, count := range c.allowCounts {
				fieldsBody = append(fieldsBody, notif.LogField{
					Name:  ip,
					Value: fmt.Sprintf("allowed %d times", count),
				})
			}
			for ip, count := range c.blockedCounts {
				fieldsBody = append(fieldsBody, notif.LogField{
					Name:  ip,
					Value: fmt.Sprintf("blocked %d times", count),
				})
			}
			notif.Notify(&notif.LogMessage{
				Level: zerolog.InfoLevel,
				Title: "ACL Summary for last " + strutils.FormatDuration(c.Notify.Interval),
				Body:  fieldsBody,
				To:    c.Notify.To,
			})
			clear(c.allowCounts)
			clear(c.blockedCounts)
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
	if c.Allow.Match(ipAndStr) {
		c.logAndNotify(ipAndStr, true)
		c.cacheRecord(ipAndStr, true)
		return true
	}
	if c.Deny.Match(ipAndStr) {
		c.logAndNotify(ipAndStr, false)
		c.cacheRecord(ipAndStr, false)
		return false
	}

	c.logAndNotify(ipAndStr, c.defaultAllow)
	c.cacheRecord(ipAndStr, c.defaultAllow)
	return c.defaultAllow
}

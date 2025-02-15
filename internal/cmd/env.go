package cmd

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/AdguardTeam/AdGuardDNS/internal/agd"
	"github.com/AdguardTeam/AdGuardDNS/internal/agdhttp"
	"github.com/AdguardTeam/AdGuardDNS/internal/debugsvc"
	"github.com/AdguardTeam/AdGuardDNS/internal/dnsdb"
	"github.com/AdguardTeam/AdGuardDNS/internal/errcoll"
	"github.com/AdguardTeam/AdGuardDNS/internal/geoip"
	"github.com/AdguardTeam/AdGuardDNS/internal/rulestat"
	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/golibs/netutil"
	env "github.com/caarlos0/env/v6"
	"github.com/getsentry/raven-go"
)

// Environment Configuration

// environments represents the configuration that is kept in the environment.
type environments struct {
	BackendEndpoint          *agdhttp.URL `env:"BACKEND_ENDPOINT,notEmpty"`
	BlockedServiceIndexURL   *agdhttp.URL `env:"BLOCKED_SERVICE_INDEX_URL,notEmpty"`
	ConsulAllowlistURL       *agdhttp.URL `env:"CONSUL_ALLOWLIST_URL,notEmpty"`
	ConsulDNSCheckKVURL      *agdhttp.URL `env:"CONSUL_DNSCHECK_KV_URL"`
	ConsulDNSCheckSessionURL *agdhttp.URL `env:"CONSUL_DNSCHECK_SESSION_URL"`
	FilterIndexURL           *agdhttp.URL `env:"FILTER_INDEX_URL,notEmpty"`
	GeneralSafeSearchURL     *agdhttp.URL `env:"GENERAL_SAFE_SEARCH_URL,notEmpty"`
	YoutubeSafeSearchURL     *agdhttp.URL `env:"YOUTUBE_SAFE_SEARCH_URL,notEmpty"`
	RuleStatURL              *agdhttp.URL `env:"RULESTAT_URL"`

	ConfPath         string `env:"CONFIG_PATH" envDefault:"./config.yml"`
	DNSDBPath        string `env:"DNSDB_PATH" envDefault:"./dnsdb.bolt"`
	FilterCachePath  string `env:"FILTER_CACHE_PATH" envDefault:"./filters/"`
	GeoIPASNPath     string `env:"GEOIP_ASN_PATH" envDefault:"./asn.mmdb"`
	GeoIPCountryPath string `env:"GEOIP_COUNTRY_PATH" envDefault:"./country.mmdb"`
	QueryLogPath     string `env:"QUERYLOG_PATH" envDefault:"./querylog.jsonl"`
	SentryDSN        string `env:"SENTRY_DSN" envDefault:"stderr"`
	SSLKeyLogFile    string `env:"SSL_KEY_LOG_FILE"`

	ListenAddr net.IP `env:"LISTEN_ADDR" envDefault:"127.0.0.1"`

	ListenPort int `env:"LISTEN_PORT" envDefault:"8181"`

	LogTimestamp strictBool `env:"LOG_TIMESTAMP" envDefault:"1"`
	LogVerbose   strictBool `env:"VERBOSE" envDefault:"0"`
}

// readEnvs reads the configuration.
func readEnvs() (envs *environments, err error) {
	envs = &environments{}
	err = env.Parse(envs)
	if err != nil {
		return nil, fmt.Errorf("parsing environments: %w", err)
	}

	return envs, nil
}

// configureLogs sets the configuration for the plain text logs.
func (envs *environments) configureLogs() {
	var flags int
	if envs.LogTimestamp {
		flags = log.LstdFlags | log.Lmicroseconds
	}

	log.SetFlags(flags)

	if envs.LogVerbose {
		log.SetLevel(log.DEBUG)
	}
}

// buildErrColl builds and returns an error collector from environment.
func (envs *environments) buildErrColl() (errColl agd.ErrorCollector, err error) {
	dsn := envs.SentryDSN
	if dsn == "stderr" {
		return errcoll.NewWriterErrorCollector(os.Stderr), nil
	}

	rc, err := raven.New(dsn)
	if err != nil {
		return nil, err
	}

	return errcoll.NewRavenErrorCollector(rc), nil
}

// buildDNSDB builds and returns an anonymous statistics collector and its
// refresh worker.
func (envs *environments) buildDNSDB(
	errColl agd.ErrorCollector,
) (d dnsdb.Interface, refr agd.Service) {
	if envs.DNSDBPath == "" {
		return dnsdb.Empty{}, agd.EmptyService{}
	}

	b := dnsdb.NewBolt(&dnsdb.BoltConfig{
		Path:    envs.DNSDBPath,
		ErrColl: errColl,
	})

	refr = agd.NewRefreshWorker(&agd.RefreshWorkerConfig{
		Context:   ctxWithDefaultTimeout,
		Refresher: b,
		ErrColl:   errColl,
		Name:      "dnsdb",
		// TODO(ameshkov): Consider making configurable.
		Interval:            15 * time.Minute,
		RefreshOnShutdown:   true,
		RoutineLogsAreDebug: false,
	})

	return b, refr
}

// geoIP returns an GeoIP database implementation from environment.
func (envs *environments) geoIP(
	c *geoIPConfig,
	errColl agd.ErrorCollector,
) (g *geoip.File, err error) {
	log.Debug("using geoip files %q and %q", envs.GeoIPASNPath, envs.GeoIPCountryPath)

	g, err = geoip.NewFile(&geoip.FileConfig{
		ErrColl:       errColl,
		ASNPath:       envs.GeoIPASNPath,
		CountryPath:   envs.GeoIPCountryPath,
		HostCacheSize: c.HostCacheSize,
		IPCacheSize:   c.IPCacheSize,
	})
	if err != nil {
		return nil, err
	}

	return g, nil
}

// debugConf returns a debug HTTP service configuration from environment.
func (envs *environments) debugConf(dnsDB dnsdb.Interface) (conf *debugsvc.Config) {
	// TODO(a.garipov): Simplify the config if these are guaranteed to always be
	// the same.
	addr := netutil.JoinHostPort(envs.ListenAddr.String(), envs.ListenPort)

	// TODO(a.garipov): Consider other ways of making the DNSDB API fully
	// optional.
	var dnsDBAddr string
	var dnsDBHdlr http.Handler
	if h, ok := dnsDB.(http.Handler); ok {
		dnsDBAddr = addr
		dnsDBHdlr = h
	} else {
		dnsDBAddr = ""
		dnsDBHdlr = http.HandlerFunc(http.NotFound)
	}

	conf = &debugsvc.Config{
		DNSDBAddr:    dnsDBAddr,
		DNSDBHandler: dnsDBHdlr,

		HealthAddr:     addr,
		PprofAddr:      addr,
		PrometheusAddr: addr,
	}

	return conf
}

// ruleStat returns a filtering rule statistics collector from environment.  It
// also returns the refresh worker service that updates the collector.
func (envs *environments) ruleStat(
	errColl agd.ErrorCollector,
) (r rulestat.Interface, refr agd.Service) {
	if envs.RuleStatURL == nil {
		log.Info("main: warning: not collecting rule stats")

		return rulestat.Empty{}, agd.EmptyService{}
	}

	httpRuleStat := rulestat.NewHTTP(&rulestat.HTTPConfig{
		URL: &envs.RuleStatURL.URL,
	})

	refr = agd.NewRefreshWorker(&agd.RefreshWorkerConfig{
		Context:   ctxWithDefaultTimeout,
		Refresher: httpRuleStat,
		ErrColl:   errColl,
		Name:      "rulestat",
		// TODO(ameshkov): Consider making configurable.
		Interval:            10 * time.Minute,
		RefreshOnShutdown:   true,
		RoutineLogsAreDebug: false,
	})

	return httpRuleStat, refr
}

// strictBool is a type for booleans that are parsed from the environment more
// strictly than the usual bool.  It only accepts "0" and "1" as valid values.
type strictBool bool

// UnmarshalText implements the encoding.TextUnmarshaler interface for
// *strictBool.
func (sb *strictBool) UnmarshalText(b []byte) (err error) {
	if len(b) == 1 {
		switch b[0] {
		case '0':
			*sb = false

			return nil
		case '1':
			*sb = true

			return nil
		default:
			// Go on and return an error.
		}
	}

	return fmt.Errorf("invalid value %q, supported: %q, %q", b, "0", "1")
}

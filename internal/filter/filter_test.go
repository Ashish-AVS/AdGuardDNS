package filter_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AdguardTeam/AdGuardDNS/internal/agd"
	"github.com/AdguardTeam/AdGuardDNS/internal/agdhttp"
	"github.com/AdguardTeam/AdGuardDNS/internal/agdtest"
	"github.com/AdguardTeam/AdGuardDNS/internal/dnsmsg"
	"github.com/AdguardTeam/golibs/testutil"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	agdtest.DiscardLogOutput(m)
}

// testFilterID is the standard ID of the filter for tests.
const testFilterID agd.FilterListID = "filter"

// testDeviceName is the standard device name for tests.  Keep in sync with
// ./testdata/filter.
const testDeviceName agd.DeviceName = "My Device"

// testSvcID is the standard ID of the blocked service for tests.
const testSvcID agd.BlockedServiceID = "service"

// testRefreshIvl is the standard refresh interval for tests.
const testRefreshIvl = 1 * time.Hour

// Common constants.  Keep in sync with ./testdata/filter and
// ./safesearchhost.csv.
const (
	blockedHost = "blocked.example.com"
	blockedFQDN = blockedHost + "."

	allowedHost = "allowed.example.com"
	allowedFQDN = allowedHost + "."

	blockedClientHost = "blocked-client.example.com"
	blockedClientFQDN = blockedClientHost + "."

	allowedClientHost = "allowed-client.example.com"
	allowedClientFQDN = allowedClientHost + "."

	blockedDeviceHost = "blocked-device.example.com"
	blockedDeviceFQDN = blockedDeviceHost + "."

	allowedDeviceHost = "allowed-device.example.com"
	allowedDeviceFQDN = allowedDeviceHost + "."

	otherNetHost = "example.net"
	otherNetFQDN = otherNetHost + "."
	otherOrgFQDN = "example.org."

	customHost = "custom.example.com"
	customFQDN = customHost + "."
	customRule = "||" + customHost + "^"

	safeSearchHost     = "duckduckgo.com"
	safeSearchRespHost = "safe.duckduckgo.com"

	safeSearchIPHost = "www.yandex.by"

	safeBrowsingHost     = "scam.example.net"
	safeBrowsingSubHost  = "subsub.sub." + safeBrowsingHost
	safeBrowsingSubFQDN  = safeBrowsingSubHost + "."
	safeBrowsingSafeHost = "safe.dns.example.net"
)

// Common immutable values.  Keep in sync with ./testdata/filter and
// ./safesearchhost.csv.
var (
	blockedIP4 = net.IP{6, 6, 6, 13}
	allowedIP4 = net.IP{7, 7, 7, 42}

	safeBrowsingSafeIP4 = net.IP{94, 140, 14, 14}

	safeSearchIPRespIP4 = net.IP{213, 180, 193, 56}
	safeSearchIPRespIP6 = net.IP{
		0x00, 0x01, 0x02, 0x03,
		0x00, 0x01, 0x02, 0x03,
		0x00, 0x01, 0x02, 0x03,
		0x00, 0x01, 0x02, 0x03,
	}
)

// Common clients.  Keep in sync with ./testdata/filter.
var (
	clientIP = netip.MustParseAddr("1.2.3.4")
	deviceIP = netip.MustParseAddr("5.6.7.8")
)

// testDataFiltersTmpl is the template for the data returned from the filter
// index.
const testDataFiltersTmpl = `{
  "filters": [
    {
      "filterId": %q,
      "downloadUrl":"http://example.com"
    }
  ]
}
`

// testDataFiltersTmpl is the template for the data returned from the blocked
// service index.
const testDataServicesTmpl = `{
  "blocked_services": [
    {
      "id": %q,
      "rules": [
        "||service.example.com^"
      ]
    }
  ]
}
`

// prepareIndex is a helper that makes a new temporary directory and places the
// testdata filter file into it as well as launches a test server with an index
// pointing to that filter, as well as serving as the blocked service index and
// safe search rules.
func prepareIndex(t testing.TB) (flts, svcs, ss *url.URL, dir string) {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", string(testFilterID)))
	require.NoError(t, err)

	var ssData []byte
	ssData, err = os.ReadFile(filepath.Join("testdata", string(agd.FilterListIDGeneralSafeSearch)))
	require.NoError(t, err)

	dir = t.TempDir()
	err = os.WriteFile(filepath.Join(dir, string(testFilterID)), b, 0o644)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pt := testutil.PanicT{}

		var werr error
		switch p := r.URL.Path; p {
		case "/services":
			_, werr = fmt.Fprintf(w, testDataServicesTmpl, testSvcID)
		case "/filters":
			_, werr = fmt.Fprintf(w, testDataFiltersTmpl, testFilterID)
		case "/safesearch":
			_, werr = w.Write(ssData)
		default:
			werr = fmt.Errorf("unexpected path %s in url", p)
		}
		require.NoError(pt, werr)
	}))
	t.Cleanup(srv.Close)

	var u *url.URL
	u, err = agdhttp.ParseHTTPURL(srv.URL)
	require.NoError(t, err)

	flts, svcs, ss = u.JoinPath("filters"), u.JoinPath("services"), u.JoinPath("safesearch")

	return flts, svcs, ss, dir
}

// newReqInfo returns a new request information structure with the given data.
func newReqInfo(
	g *agd.FilteringGroup,
	p *agd.Profile,
	host string,
	ip netip.Addr,
	qt dnsmsg.RRType,
) (ri *agd.RequestInfo) {
	var d *agd.Device
	if p != nil {
		d = &agd.Device{
			Name:             testDeviceName,
			FilteringEnabled: true,
		}
	}

	ri = &agd.RequestInfo{
		Device:         d,
		Profile:        p,
		FilteringGroup: g,
		Messages: &dnsmsg.Constructor{
			FilteredResponseTTL: 10 * time.Second,
		},
		Host:     host,
		RemoteIP: ip,
		QType:    qt,
	}

	return ri
}

package dnssvc

import (
	"context"
	"net/url"
	"testing"

	"github.com/AdguardTeam/AdGuardDNS/internal/agd"
	"github.com/AdguardTeam/AdGuardDNS/internal/dnsserver"
	"github.com/AdguardTeam/golibs/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_Wrap_deviceID(t *testing.T) {
	testCases := []struct {
		name         string
		cliSrvName   string
		wantDeviceID agd.DeviceID
		wantErrMsg   string
		wildcards    []string
		proto        agd.Protocol
	}{{
		name:         "udp",
		cliSrvName:   "",
		wantDeviceID: "",
		wantErrMsg:   "",
		wildcards:    nil,
		proto:        agd.ProtoDNSUDP,
	}, {
		name:         "tls_no_device_id",
		cliSrvName:   "dns.example.com",
		wantDeviceID: "",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoT,
	}, {
		name:         "tls_no_client_server_name",
		cliSrvName:   "",
		wantDeviceID: "",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoT,
	}, {
		name:         "tls_device_id",
		cliSrvName:   "dev.dns.example.com",
		wantDeviceID: "dev",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoT,
	}, {
		name:         "tls_bad_device_id",
		cliSrvName:   "!!!.dns.example.com",
		wantDeviceID: "",
		wantErrMsg: `tls server name device id check: bad device id "!!!": ` +
			`bad domain name label rune '!'`,
		wildcards: []string{"*.dns.example.com"},
		proto:     agd.ProtoDoT,
	}, {
		name:         "tls_deep_subdomain",
		cliSrvName:   "abc.def.dns.example.com",
		wantDeviceID: "",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoT,
	}, {
		name: "tls_device_id_too_long",
		cliSrvName: `abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmno` +
			`pqrstuvwxyz0123456789.dns.example.com`,
		wantDeviceID: "",
		wantErrMsg: `tls server name device id check: bad device id ` +
			`"abcdefghijklmnopqrstuvwxyz0123456789` +
			`abcdefghijklmnopqrstuvwxyz0123456789": ` +
			`too long: got 72 bytes, max 8`,
		wildcards: []string{"*.dns.example.com"},
		proto:     agd.ProtoDoT,
	}, {
		name:         "quic_device_id",
		cliSrvName:   "dev.dns.example.com",
		wantDeviceID: "dev",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoQ,
	}, {
		name:         "tls_device_id_suffix",
		cliSrvName:   "dev.mydns.example.com",
		wantDeviceID: "",
		wantErrMsg:   "",
		wildcards:    []string{"*.dns.example.com"},
		proto:        agd.ProtoDoT,
	}, {
		name:         "tls_device_id_subdomain_wildcard",
		cliSrvName:   "dev.sub.dns.example.com",
		wantDeviceID: "dev",
		wantErrMsg:   "",
		wildcards: []string{
			"*.dns.example.com",
			"*.sub.dns.example.com",
		},
		proto: agd.ProtoDoT,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			ctx = dnsserver.ContextWithClientInfo(ctx, dnsserver.ClientInfo{
				TLSServerName: tc.cliSrvName,
			})

			deviceID, err := deviceIDFromContext(ctx, tc.proto, tc.wildcards)
			assert.Equal(t, tc.wantDeviceID, deviceID)
			testutil.AssertErrorMsg(t, tc.wantErrMsg, err)
		})
	}
}

func TestService_Wrap_deviceIDHTTPS(t *testing.T) {
	testCases := []struct {
		name         string
		path         string
		wantDeviceID agd.DeviceID
		wantErrMsg   string
	}{{
		name:         "no_device_id",
		path:         "/dns-query",
		wantDeviceID: "",
		wantErrMsg:   "",
	}, {
		name:         "no_device_id_slash",
		path:         "/dns-query/",
		wantDeviceID: "",
		wantErrMsg:   "",
	}, {
		name:         "device_id",
		path:         "/dns-query/cli",
		wantDeviceID: "cli",
		wantErrMsg:   "",
	}, {
		name:         "device_id_slash",
		path:         "/dns-query/cli/",
		wantDeviceID: "cli",
		wantErrMsg:   "",
	}, {
		name:         "bad_url",
		path:         "/foo",
		wantDeviceID: "",
		wantErrMsg:   `http url device id check: bad path "/foo"`,
	}, {
		name:         "extra",
		path:         "/dns-query/cli/foo",
		wantDeviceID: "",
		wantErrMsg: `http url device id check: bad path "/dns-query/cli/foo": ` +
			`extra parts`,
	}, {
		name:         "bad_device_id",
		path:         "/dns-query/!!!",
		wantDeviceID: "",
		wantErrMsg: `http url device id check: bad device id "!!!": ` +
			`bad domain name label rune '!'`,
	}}

	const proto = agd.ProtoDoH

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			u := &url.URL{
				Scheme: "https",
				Host:   "dns.example.com",
				Path:   tc.path,
			}

			ctx := context.Background()
			ctx = dnsserver.ContextWithClientInfo(ctx, dnsserver.ClientInfo{
				URL: u,
			})

			deviceID, err := deviceIDFromContext(ctx, proto, nil)
			assert.Equal(t, tc.wantDeviceID, deviceID)
			testutil.AssertErrorMsg(t, tc.wantErrMsg, err)
		})
	}

	t.Run("domain_name", func(t *testing.T) {
		const want = "dev"

		u := &url.URL{
			Scheme: "https",
			Host:   want + ".dns.example.com",
			Path:   "/dns-query",
		}

		ctx := context.Background()
		ctx = dnsserver.ContextWithClientInfo(ctx, dnsserver.ClientInfo{
			URL:           u,
			TLSServerName: u.Host,
		})
		ctx = dnsserver.ContextWithServerInfo(ctx, dnsserver.ServerInfo{
			Proto: proto,
		})

		deviceID, err := deviceIDFromContext(ctx, proto, []string{"*.dns.example.com"})
		require.NoError(t, err)

		assert.Equal(t, agd.DeviceID(want), deviceID)
	})
}

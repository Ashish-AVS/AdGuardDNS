package agd

import (
	"crypto/tls"
	"net/netip"

	"github.com/AdguardTeam/golibs/stringutil"
	"github.com/ameshkov/dnscrypt/v2"
	"github.com/miekg/dns"
)

// Servers And Server Groups

// ServerGroup is a group of DNS servers all of which use the same filtering
// settings.
type ServerGroup struct {
	// TLS are the TLS settings for this server group, if any.
	TLS *TLS

	// DDR is the configuration for the server group's Discovery Of Designated
	// Resolvers (DDR) handlers.  DDR is never nil.
	DDR *DDR

	// Name is the unique name of the server group.
	Name ServerGroupName

	// FilteringGroup is the ID of the filtering group for this server.
	FilteringGroup FilteringGroupID

	// Servers are the settings for servers.  Each element must be non-nil.
	Servers []*Server
}

// ServerGroupName is the name of a server group.
type ServerGroupName string

// TLS is the TLS configuration of a DNS server group.
type TLS struct {
	// Conf is the server's TLS configuration.
	Conf *tls.Config

	// DeviceIDWildcards are the domain wildcards used to detect device IDs from
	// clients' server names.
	DeviceIDWildcards []string

	// SessionKeys are paths to files containing the TLS session keys for this
	// server.
	SessionKeys []string
}

// DDR is the configuration for the server group's Discovery Of Designated
// Resolvers (DDR) handlers.
type DDR struct {
	// DeviceTargets is the set of all domain names, subdomains of which should
	// be checked for DDR queries with device IDs.
	DeviceTargets *stringutil.Set

	// PublicTargets is the set of all public domain names, DDR queries for
	// which should be processed.
	PublicTargets *stringutil.Set

	// DeviceRecordTemplates are used to respond to DDR queries from recognized
	// devices.
	DeviceRecordTemplates []*dns.SVCB

	// PubilcRecordTemplates are used to respond to DDR queries from
	// unrecognized devices.
	PublicRecordTemplates []*dns.SVCB

	// Enabled shows if DDR queries are processed.  If it is false, DDR domain
	// name queries receive an NXDOMAIN response.
	Enabled bool
}

// Server represents a single DNS server.  That is, an entity that binds to one
// or more ports and serves DNS over a single protocol.
type Server struct {
	// DNSCrypt are the DNSCrypt settings for this server, if any.
	DNSCrypt *DNSCryptConfig

	// TLS is the TLS configuration for this server, if any.
	TLS *tls.Config

	// Name is the unique name of the server.  Not to be confused with a TLS
	// Server Name.
	Name ServerName

	// BindAddresses are addresses this server binds to.
	BindAddresses []netip.AddrPort

	// Protocol is the protocol of the server.
	Protocol Protocol

	// LinkedIPEnabled shows if the linked IP addresses should be used to detect
	// profiles on this server.
	LinkedIPEnabled bool
}

// ServerName is the name of a server.
type ServerName string

// DNSCryptConfig is the DNSCrypt configuration of a DNS server.
type DNSCryptConfig struct {
	// Cert is the DNSCrypt certificate.
	Cert *dnscrypt.Cert

	// ProviderName is the name of the DNSCrypt provider.
	ProviderName string
}

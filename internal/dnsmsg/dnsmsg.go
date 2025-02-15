// Package dnsmsg contains common constants, functions, and types for
// inspecting and constructing DNS messages.
//
// TODO(a.garipov): Consider moving all or some of this stuff to module golibs.
package dnsmsg

import (
	"fmt"
	"net/netip"

	"github.com/AdguardTeam/AdGuardDNS/internal/agdnet"
	"github.com/miekg/dns"
)

// Common Constants, Types, And Utilities

// RCode is a semantic alias for uint8 values when they are used as a DNS
// response code RCODE.
type RCode = uint8

// RRType is a semantic alias for uint16 values when they are used as a DNS
// resource record (RR) type.
type RRType = uint16

// Class is a semantic alias for uint16 values when they are used as a DNS class
// code.
type Class = uint16

// DefaultEDNSUDPSize is the default size used for EDNS content.
//
// See https://datatracker.ietf.org/doc/html/rfc6891#section-6.2.5.
const DefaultEDNSUDPSize = 4096

// MaxTXTStringLen is the maximum length of a single string within a TXT
// resource record.
//
// See also https://datatracker.ietf.org/doc/html/rfc6763#section-6.1.
const MaxTXTStringLen int = 255

// Clone returns a new *Msg which is a deep copy of msg.  Use this instead of
// msg.Copy, because the latter does not actually produce a deep copy of msg.
//
// See https://github.com/miekg/dns/issues/1351.
//
// TODO(a.garipov): See if we can also decrease allocations for such cases by
// modifying more of the original code.
func Clone(msg *dns.Msg) (clone *dns.Msg) {
	if msg == nil {
		return nil
	}

	clone = msg.Copy()

	// Make sure that nilness of the RR slices is retained.
	if msg.Answer == nil {
		clone.Answer = nil
	}

	if msg.Ns == nil {
		clone.Ns = nil
	}

	if msg.Extra == nil {
		clone.Extra = nil
	}

	return clone
}

// IsDO returns true if msg has an EDNS option pseudosection and that
// pseudosection has the DNSSEC OK (DO) bit set.
func IsDO(msg *dns.Msg) (ok bool) {
	opt := msg.IsEdns0()

	return opt != nil && opt.Do()
}

// ECSFromMsg returns the EDNS Client Subnet option information from msg, if
// any.  If there is none, it returns netip.Prefix{}.  msg must not be nil.  err
// is not nil only if msg contains a malformed EDNS Client Subnet option or the
// address family is unsupported (that is, neither IPv4 nor IPv6).  Any error
// returned from ECSFromMsg will have the underlying type of BadECSError.
func ECSFromMsg(msg *dns.Msg) (subnet netip.Prefix, scope uint8, err error) {
	opt := msg.IsEdns0()
	if opt == nil {
		return netip.Prefix{}, 0, nil
	}

	for _, opt := range opt.Option {
		esn, ok := opt.(*dns.EDNS0_SUBNET)
		if !ok {
			continue
		}

		subnet, scope, err = ecsData(esn)
		if err != nil {
			return netip.Prefix{}, 0, BadECSError{Err: err}
		} else if subnet != (netip.Prefix{}) {
			return subnet, scope, nil
		}
	}

	return netip.Prefix{}, 0, nil
}

// ecsData returns the subnet and scope information from an EDNS Client Subnet
// option.  It returns an error if esn does not contain valid, RFC-compliant
// EDNS Client Subnet information or the address family is unsupported.
func ecsData(esn *dns.EDNS0_SUBNET) (subnet netip.Prefix, scope uint8, err error) {
	fam := agdnet.AddrFamily(esn.Family)
	if fam != agdnet.AddrFamilyIPv4 && fam != agdnet.AddrFamilyIPv6 {
		return netip.Prefix{}, 0, fmt.Errorf("unsupported addr family number %d", fam)
	}

	ip, err := agdnet.IPToAddr(esn.Address, fam)
	if err != nil {
		return netip.Prefix{}, 0, fmt.Errorf("bad ecs ip addr: %w", err)
	}

	prefixLen := int(esn.SourceNetmask)
	subnet = netip.PrefixFrom(ip, prefixLen)
	if !subnet.IsValid() {
		return netip.Prefix{}, 0, fmt.Errorf(
			"bad src netmask %d for addr family %s",
			prefixLen,
			fam,
		)
	}

	// Make sure that the subnet address does not have any bits beyond the given
	// prefix set to one.
	//
	// See https://datatracker.ietf.org/doc/html/rfc7871#section-6.
	if subnet.Masked() != subnet {
		return netip.Prefix{}, 0, fmt.Errorf(
			"ip %s has non-zero bits beyond prefix %d",
			ip,
			prefixLen,
		)
	}

	return subnet, esn.SourceScope, nil
}

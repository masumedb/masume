package cfg

import (
	"net"
	"strconv"
	"strings"

	"github.com/masumedb/masume/internal/core"
)

// takesOtherHosts is true for an engine whose driver takes several hosts.
func takesOtherHosts(engine core.Engine) bool {
	family := core.ResolveEngineInfo(engine).Family
	return family == core.FamilyPostgres || family == core.FamilyMongo
}

// splitHostList reads hosts joined by commas, each with an optional port, such as
// `db1:5432,db2,[::1]:5433`. A host without a port takes the default port.
func splitHostList(written string, defaultPort int) ([]string, []int, error) {
	hosts, ports := []string{}, []int{}
	for _, part := range strings.Split(written, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, nil, failTarget("the host list %q has an empty entry", written)
		}
		host, port := strings.Trim(part, "[]"), defaultPort
		if split, writtenPort, err := net.SplitHostPort(part); err == nil {
			read, portErr := readPortNumber(writtenPort)
			if portErr != nil {
				return nil, nil, portErr
			}
			host, port = split, read
		}
		hosts, ports = append(hosts, host), append(ports, port)
	}
	return hosts, ports, nil
}

// joinHosts returns the hosts as `host:port` entries, or nothing for no host.
func joinHosts(hosts []string, ports []int) []string {
	if len(hosts) == 0 {
		return nil
	}
	joined := make([]string, 0, len(hosts))
	for at := range hosts {
		joined = append(joined, net.JoinHostPort(hosts[at], strconv.Itoa(ports[at])))
	}
	return joined
}

// joinOtherHosts returns the hosts after the first one as `host:port` entries.
func joinOtherHosts(hosts []string, ports []int) []string {
	if len(hosts) < 2 {
		return nil
	}
	return joinHosts(hosts[1:], ports[1:])
}

// findOtherHostsProblem returns why the profile cannot use its other hosts, or nothing.
func findOtherHostsProblem(profile Profile) string {
	switch {
	case len(profile.OtherHosts) == 0:
		return ""
	case !takesOtherHosts(profile.Engine):
		return "several hosts are for the PostgreSQL and MongoDB families only"
	case profile.OpensTunnel():
		return "several hosts cannot be used with an ssh tunnel"
	case profile.SRV:
		return "several hosts cannot be used with srv"
	}
	return ""
}

// hostListPlaceholder stands in for a host list while the rest of a URL is parsed.
const hostListPlaceholder = "hosts.invalid"

// takeHostList returns the URL with a host list replaced by one placeholder host, and the
// list. The URL parser reads one host only. A URL with one host comes back unchanged.
func takeHostList(text string) (string, string) {
	start := strings.Index(text, "://")
	if start < 0 {
		return text, ""
	}
	start += len("://")
	end := len(text)
	if at := strings.IndexAny(text[start:], "/?#"); at >= 0 {
		end = start + at
	}
	if at := strings.LastIndex(text[start:end], "@"); at >= 0 {
		start += at + 1
	}
	hosts := text[start:end]
	if !strings.Contains(hosts, ",") {
		return text, ""
	}
	return text[:start] + hostListPlaceholder + text[end:], hosts
}

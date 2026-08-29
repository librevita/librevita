package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

func clientIPExtractor(trustedProxies string) echo.IPExtractor {
	if strings.TrimSpace(trustedProxies) == "" {
		return remoteAddrIP
	}
	var trust []echo.TrustOption
	for _, p := range strings.Split(trustedProxies, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ipnet, err := net.ParseCIDR(p); err == nil {
			trust = append(trust, echo.TrustIPRange(ipnet))
			continue
		}
		if ip := net.ParseIP(p); ip != nil {
			trust = append(trust, echo.TrustIPRange(hostIPNet(ip)))
		}
	}
	return echo.ExtractIPFromXFFHeader(trust...)
}

func hostIPNet(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)}
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
}

func remoteAddrIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

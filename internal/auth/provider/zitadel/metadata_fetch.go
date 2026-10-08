package zitadel

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"

	"isekai-mcp-gateway/internal/config"
	"isekai-mcp-gateway/internal/failure"
)

// Resolve and validate every destination immediately before dialing. Dialing the
// validated numeric address closes the DNS-rebinding gap between check and use.
func publicAddress(ip netip.Addr) bool {
	// 先排除私有、回环、链路本地等非全局单播地址
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}

	// 再排除保留网段，防止通过特殊地址绕过 SSRF 防护
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		if netip.MustParsePrefix(cidr).Contains(ip) {
			return false
		}
	}
	return true
}

// metadataURL 仅接受公网 HTTPS、443 端口且无用户信息或片段的 URL
func metadataURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Port() != "" && u.Port() != "443" {
		return errors.New("CIMD metadata requires a public HTTPS URL on port 443")
	}
	return nil
}

func fetchMetadata(ctx context.Context, raw string, g config.Gateway) ([]byte, error) {
	if err := metadataURL(raw); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		Proxy:               nil,
		TLSHandshakeTimeout: time.Duration(g.Fetch.TimeoutMS) * time.Millisecond,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// 拨号前强制校验端口，并解析为具体 IP 后再连接，封堵 DNS 重绑定
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if port != strconv.Itoa(443) {
				return nil, errors.New("metadata port is prohibited")
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, failure.Dependency("metadata.dns", err)
			}
			if len(addresses) == 0 {
				return nil, errors.New("metadata DNS lookup failed")
			}

			// 任一解析结果落入禁止网段即整体拒绝
			for _, ip := range addresses {
				if !publicAddress(ip) {
					return nil, errors.New("metadata DNS includes a prohibited address")
				}
			}
			var last error
			for _, ip := range addresses {
				conn, e := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if e == nil {
					return conn, nil
				}
				last = e
			}
			return nil, last
		},
	}
	defer transport.CloseIdleConnections()
	h := &http.Client{Transport: transport, Timeout: time.Duration(g.Fetch.TimeoutMS) * time.Millisecond,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			// 限制重定向次数，且每次跳转目标都必须重新通过 URL 校验
			if len(via) > g.Fetch.MaxRedirects {
				return errors.New("too many metadata redirects")
			}
			return metadataURL(r.URL.String())
		},
	}
	r, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	res, err := h.Do(r)
	if err != nil {
		return nil, failure.Dependency("metadata.fetch", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, &failure.Error{Code: failure.Upstream, Message: "Client metadata endpoint failed.", Op: "metadata.response", UpstreamStatus: res.StatusCode, Retryable: res.StatusCode == 429 || res.StatusCode >= 500}
	}
	// 多读 1 字节用于判断是否超出上限，避免无界读取
	b, err := io.ReadAll(io.LimitReader(res.Body, g.Fetch.MaxBytes+1))
	if err != nil {
		return nil, failure.Dependency("metadata.read", err)
	}
	if int64(len(b)) > g.Fetch.MaxBytes {
		return nil, errors.New("client metadata exceeds limit")
	}
	return b, nil
}

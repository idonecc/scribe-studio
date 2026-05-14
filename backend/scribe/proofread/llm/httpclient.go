// SPDX-License-Identifier: GPL-3.0-or-later
package llm

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// BuildHTTPClient returns an *http.Client configured to route through
// proxyURL, if supplied. proxyURL accepts the same shape users plug
// into Clash / V2Ray / Shadowsocks:
//
//	http://127.0.0.1:7890
//	https://user:pass@example:8443
//	socks5://127.0.0.1:7891
//	socks5h://127.0.0.1:7891  (DNS resolved by the proxy)
//
// Empty proxyURL ⇒ a plain client with the same timeout. This exists
// because Chinese users typically can't reach generativelanguage.
// googleapis.com (Gemini), api.openai.com, and so on without a local
// VPN forwarder.
func BuildHTTPClient(proxyURL string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return &http.Client{Timeout: timeout}, nil
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid proxy URL: missing scheme or host")
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		transport := &http.Transport{
			Proxy:                 http.ProxyURL(u),
			DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// Leave InsecureSkipVerify off — users routing through a
			// HTTPS proxy still expect cert validation against the
			// upstream LLM host.
			TLSClientConfig: &tls.Config{},
		}
		return &http.Client{Transport: transport, Timeout: timeout}, nil

	case "socks5", "socks5h":
		// proxy.FromURL handles auth in the URL (socks5://user:pass@host:port).
		dialer, err := proxy.FromURL(u, &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second})
		if err != nil {
			return nil, fmt.Errorf("socks5 dialer: %w", err)
		}
		ctxDialer, ok := dialer.(proxy.ContextDialer)
		dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ok {
				return ctxDialer.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		}
		transport := &http.Transport{
			DialContext:           dialContext,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       &tls.Config{},
		}
		return &http.Client{Transport: transport, Timeout: timeout}, nil
	}

	return nil, fmt.Errorf("unsupported proxy scheme %q (use http://, https://, socks5:// or socks5h://)", u.Scheme)
}

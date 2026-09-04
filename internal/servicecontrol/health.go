package servicecontrol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// HealthChecker verifies only compile-time service health endpoints supplied by
// Policy. Request data never controls the destination URL.
type HealthChecker interface {
	Check(context.Context, string) error
}

type HTTPHealth struct {
	Client *http.Client
}

func (h HTTPHealth) Check(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" {
		return errors.New("invalid service health URL")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("service health URL must use a loopback address")
	}
	client := h.Client
	if client == nil {
		transport := &http.Transport{
			Proxy:               nil,
			DisableCompression:  true,
			MaxIdleConns:        2,
			IdleConnTimeout:     5 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
		}
		client = &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("service health redirects are not allowed")
			},
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return errors.New("create service health request")
	}
	req.Header.Set("User-Agent", "Quantum-Control/health-probe")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("service health endpoint is unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("service health endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

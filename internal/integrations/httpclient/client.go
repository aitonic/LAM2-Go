// Package httpclient applies the Go project's httpproxy rules per profile.
// A bounded cache reuses transports without ever changing process-wide proxy env.
package httpclient

import (
	"encoding/json"
	"golang.org/x/net/http/httpproxy"
	"lam.local/lam2/internal/account"
	"lam.local/lam2/internal/model"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Pool struct {
	mu      sync.Mutex
	clients map[string]*http.Client
	order   []string
}

func New() *Pool { return &Pool{clients: map[string]*http.Client{}} }
func (p *Pool) Client(proxy model.Proxy) (*http.Client, error) {
	if proxy.Mode == "" {
		proxy.Mode = "inherit"
	}
	if e := account.ValidateProxy(proxy); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(proxy)
	key := string(raw)
	p.mu.Lock()
	defer p.mu.Unlock()
	if c := p.clients[key]; c != nil {
		return c, nil
	}
	cfg := &httpproxy.Config{}
	switch proxy.Mode {
	case "inherit":
		cfg = httpproxy.FromEnvironment()
	case "manual":
		cfg.HTTPProxy = proxy.HTTP
		cfg.HTTPSProxy = proxy.HTTPS
		cfg.NoProxy = proxy.NoProxy
		if cfg.HTTPProxy == "" {
			cfg.HTTPProxy = proxy.All
		}
		if cfg.HTTPSProxy == "" {
			cfg.HTTPSProxy = proxy.All
		}
	}
	selectProxy := cfg.ProxyFunc()
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = func(r *http.Request) (*url.URL, error) { return selectProxy(r.URL) }
	tr.MaxIdleConns = 64
	tr.MaxIdleConnsPerHost = 8
	tr.IdleConnTimeout = 90 * time.Second
	c := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if len(p.order) >= 32 {
		old := p.order[0]
		p.order = p.order[1:]
		p.clients[old].CloseIdleConnections()
		delete(p.clients, old)
	}
	p.clients[key] = c
	p.order = append(p.order, key)
	return c, nil
}
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		c.CloseIdleConnections()
	}
	p.clients = map[string]*http.Client{}
	p.order = nil
}

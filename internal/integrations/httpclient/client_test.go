package httpclient

import (
	"lam.local/lam2/internal/model"
	"net/http"
	"testing"
)

func TestProfileProxyIsolation(t *testing.T) {
	p := New()
	defer p.Close()
	direct, e := p.Client(model.Proxy{Mode: "disabled"})
	if e != nil {
		t.Fatal(e)
	}
	proxied, e := p.Client(model.Proxy{Mode: "manual", All: "http://127.0.0.1:9999", NoProxy: ".example.org"})
	if e != nil {
		t.Fatal(e)
	}
	req, _ := http.NewRequest("GET", "https://api.example.com/v1", nil)
	a, e := direct.Transport.(*http.Transport).Proxy(req)
	if e != nil || a != nil {
		t.Fatal(a, e)
	}
	b, e := proxied.Transport.(*http.Transport).Proxy(req)
	if e != nil || b == nil || b.Host != "127.0.0.1:9999" {
		t.Fatal(b, e)
	}
	req, _ = http.NewRequest("GET", "https://api.example.org/v1", nil)
	b, e = proxied.Transport.(*http.Transport).Proxy(req)
	if e != nil || b != nil {
		t.Fatal(b, e)
	}
}

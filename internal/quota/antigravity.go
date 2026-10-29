package quota

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"lam.local/lam2/internal/storage"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Antigravity struct {
	URL           string
	Port          int
	CSRF          model.CredentialRef
	Secrets       storage.Secrets
	Client        *http.Client
	ScanProcesses func(context.Context) ([]byte, error)
	Cache         *AntigravityCache
	Force         bool
}

func unavailableAntigravity() model.QuotaSnapshot {
	return model.QuotaSnapshot{ProfileID: "antigravity", Source: "antigravity-local", Status: "unavailable", Windows: []model.QuotaWindow{}}
}
func (a Antigravity) Read(ctx context.Context) (model.QuotaSnapshot, error) {
	bad := unavailableAntigravity()
	if a.Port < 0 || a.Port > 65535 {
		return bad, fault.New("ANTIGRAVITY_INVALID", "Invalid configured Antigravity port")
	}
	rawURL := a.URL
	if rawURL == "" {
		if a.Port == 0 {
			return bad, fault.New("ANTIGRAVITY_PORT_REQUIRED", "Set the Antigravity language-server port in Settings")
		}
		rawURL = "https://127.0.0.1:" + strconv.Itoa(a.Port)
	}
	u, e := url.Parse(rawURL)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return bad, fault.New("ANTIGRAVITY_INVALID", "Use a local Antigravity endpoint without credentials")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return bad, fault.New("ANTIGRAVITY_INVALID", "Antigravity endpoint must use an explicit loopback IP")
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 {
		return bad, fault.New("ANTIGRAVITY_PORT_REQUIRED", "Set an explicit Antigravity language-server port")
	}
	var processes []AntigravityProcess
	if a.CSRF.Kind != "" {
		if a.Secrets == nil {
			return bad, fault.New("ANTIGRAVITY_AUTH", "System credential store unavailable")
		}
		token, e := a.Secrets.Get(ctx, a.CSRF)
		if e != nil {
			return bad, fault.New("ANTIGRAVITY_AUTH", "Configured local-service credential is unavailable")
		}
		processes = []AntigravityProcess{{token: token}}
	} else {
		scan := a.ScanProcesses
		if scan == nil {
			scan = scanAntigravityProcesses
		}
		raw, e := scan(ctx)
		if e != nil {
			return bad, fault.New("ANTIGRAVITY_PROCESS_NOT_FOUND", "Cannot inspect the local language-server process")
		}
		processes = ParseAntigravityProcesses(raw)
		clear(raw)
		if len(processes) == 0 {
			return bad, fault.New("ANTIGRAVITY_PROCESS_NOT_FOUND", "Antigravity language server is not running")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var last error
	for _, process := range processes {
		if process.token == "" || len(process.token) > 4096 || strings.ContainsAny(process.token, "\r\n\x00") {
			last = fault.New("ANTIGRAVITY_AUTH", "Invalid local-service authentication")
			continue
		}
		sum := sha256.Sum256([]byte(process.token))
		key := u.Host + ":" + strconv.Itoa(process.PID) + ":" + hex.EncodeToString(sum[:])
		fetch := func() (model.QuotaSnapshot, error) {
			q, e := a.fetch(ctx, *u, process.token)
			q.LocalPort = port
			q.LocalProcessID = process.PID
			return q, e
		}
		var q model.QuotaSnapshot
		if a.Cache != nil {
			q, e = a.Cache.read(ctx, key, a.Force, fetch)
		} else {
			q, e = fetch()
		}
		if e == nil {
			return q, nil
		}
		last = e
		if !q.FetchedAt.IsZero() {
			bad = q
		}
		if ctx.Err() != nil {
			break
		}
	}
	if last == nil {
		last = fault.New("ANTIGRAVITY_UNAVAILABLE", "Local language server did not report quota")
	}
	bad.ErrorCode = fault.Code(last)
	if !bad.FetchedAt.IsZero() {
		bad.Stale = true
		bad.Status = "stale"
	}
	return bad, last
}
func (a Antigravity) fetch(ctx context.Context, u url.URL, token string) (model.QuotaSnapshot, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	// Antigravity uses a self-signed certificate. The destination has already
	// been restricted to a literal loopback IP; redirects and proxies are off.
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} // local language server only
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if a.Client != nil {
		cp := *a.Client
		cp.CheckRedirect = client.CheckRedirect
		client = &cp
	}
	defer transport.CloseIdleConnections()
	fetch := func(method string) ([]byte, error) {
		schemes := []string{u.Scheme}
		if u.Scheme == "https" {
			schemes = append(schemes, "http")
		}
		for _, scheme := range schemes {
			endpoint := u
			endpoint.Scheme = scheme
			endpoint.Path = "/exa.language_server_pb.LanguageServerService/" + method
			req, e := http.NewRequestWithContext(ctx, "POST", endpoint.String(), bytes.NewBufferString("{}"))
			if e != nil {
				return nil, e
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			req.Header.Set("X-Codeium-Csrf-Token", token)
			r, e := client.Do(req)
			if e != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			b, re := io.ReadAll(io.LimitReader(r.Body, (4<<20)+1))
			r.Body.Close()
			if r.StatusCode == 200 && re == nil && len(b) <= 4<<20 && json.Valid(b) {
				return b, nil
			}
		}
		return nil, fault.New("ANTIGRAVITY_UNAVAILABLE", "Local Antigravity quota request failed")
	}
	models, me := fetch("GetUserStatus")
	summary, se := fetch("RetrieveUserQuotaSummary")
	if me != nil && se != nil {
		return unavailableAntigravity(), me
	}
	q, e := ParseAntigravity(models, summary, time.Now().UTC(), 3*time.Minute)
	if e == nil && (me != nil || se != nil) {
		q.Status = "partial"
	}
	return q, e
}
func ParseAntigravity(models, summary []byte, now time.Time, ttl time.Duration) (model.QuotaSnapshot, error) {
	q := unavailableAntigravity()
	q.FetchedAt = now
	q.ExpiresAt = now.Add(ttl)
	var groups struct {
		Response struct {
			Description string
			Groups      []model.AntigravityGroup
		}
	}
	if json.Unmarshal(summary, &groups) == nil {
		q.Description = groups.Response.Description
		for _, g := range groups.Response.Groups {
			if g.Name != "" && len(g.Buckets) > 0 {
				q.Groups = append(q.Groups, g)
			}
		}
	}
	type config struct {
		Label string
		Quota struct {
			Remaining *float64 `json:"remainingFraction"`
			Reset     string   `json:"resetTime"`
		} `json:"quotaInfo"`
	}
	type data struct {
		Models []config `json:"clientModelConfigs"`
	}
	var m struct {
		User struct {
			Data data `json:"cascadeModelConfigData"`
			Plan struct {
				PlanName string `json:"planName"`
			} `json:"planStatus"`
		} `json:"userStatus"`
		Data data `json:"cascadeModelConfigData"`
	}
	if json.Unmarshal(models, &m) == nil {
		items := m.User.Data.Models
		if len(items) == 0 {
			items = m.Data.Models
		}
		q.PlanType = m.User.Plan.PlanName
		for _, v := range items {
			if v.Label != "" {
				q.Models = append(q.Models, model.AntigravityModel{Label: v.Label, RemainingFraction: validFraction(v.Quota.Remaining), ResetTime: v.Quota.Reset})
			}
		}
	}
	add := func(id string, fraction *float64, reset string, window string) {
		w := model.QuotaWindow{ID: id}
		if f := validFraction(fraction); f != nil {
			remain := *f * 100
			used := 100 - remain
			w.RemainingPercent = &remain
			w.UsedPercent = &used
		}
		if t, e := time.Parse(time.RFC3339, reset); e == nil {
			w.ResetsAt = &t
		}
		switch strings.ToLower(window) {
		case "weekly", "week", "seven_day":
			w.WindowMinutes = 10080
		case "five_hour", "5h", "five_hours":
			w.WindowMinutes = 300
		}
		q.Windows = append(q.Windows, w)
	}
	if len(q.Groups) > 0 {
		for i := range q.Groups {
			for j := range q.Groups[i].Buckets {
				b := &q.Groups[i].Buckets[j]
				b.RemainingFraction = validFraction(b.RemainingFraction)
				if !b.Disabled {
					add(q.Groups[i].Name+"/"+b.Name, b.RemainingFraction, b.ResetTime, b.Window)
				}
			}
		}
	} else {
		for _, v := range q.Models {
			add(v.Label, v.RemainingFraction, v.ResetTime, "")
		}
	}
	if len(q.Groups) == 0 && len(q.Models) == 0 {
		return q, fault.New("ANTIGRAVITY_UNAVAILABLE", "No local quota groups or model details were reported")
	}
	q.Status = "ready"
	return q, nil
}
func validFraction(v *float64) *float64 {
	if v == nil || *v < 0 || *v > 1 {
		return nil
	}
	return v
}

// Redacted formatter for debug logs. Local process credentials never persist.
func (p AntigravityProcess) String() string {
	return fmt.Sprintf("AntigravityProcess{PID:%d Standalone:%t}", p.PID, p.Standalone)
}

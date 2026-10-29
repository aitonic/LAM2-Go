package quota

import (
	"bytes"
	"context"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AntigravityProcess struct {
	PID        int
	Standalone bool
	token      string
}

func ParseAntigravityProcesses(raw []byte) []AntigravityProcess {
	out := []AntigravityProcess{}
	seen := map[int]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, e := strconv.Atoi(fields[0])
		if e != nil || pid <= 0 || seen[pid] {
			continue
		}
		cmd := strings.Join(fields[1:], " ")
		lower := strings.ToLower(cmd)
		if !strings.Contains(lower, "language_server") {
			continue
		}
		token := ""
		for i, field := range fields[1:] {
			if strings.HasPrefix(field, "--csrf_token=") {
				token = strings.TrimPrefix(field, "--csrf_token=")
				break
			}
			if field == "--csrf_token" && i+2 < len(fields) {
				token = fields[i+2]
				break
			}
		}
		token = strings.Trim(token, "\"'")
		if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n\x00") {
			continue
		}
		standalone := strings.Contains(lower, "--standalone") || strings.Contains(lower, "/antigravity.app/") || (strings.Contains(lower, "--app_data_dir antigravity ") && !strings.Contains(lower, "--app_data_dir antigravity-ide"))
		out = append(out, AntigravityProcess{pid, standalone, token})
		seen[pid] = true
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Standalone && !out[j].Standalone })
	return out
}

type processOutput struct {
	bytes.Buffer
	limit int
}

func (b *processOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fault.New("PROCESS_SCAN_LIMIT", "Process list exceeded limit")
	}
	return b.Buffer.Write(p)
}
func scanAntigravityProcesses(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-ww", "-eo", "pid,args")
	out := &processOutput{limit: 4 << 20}
	cmd.Stdout = out
	if e := cmd.Run(); e != nil {
		clear(out.Bytes())
		return nil, fault.New("PROCESS_SCAN_FAILED", "Cannot inspect local language-server process")
	}
	return out.Bytes(), nil
}

type antiFlight struct {
	done chan struct{}
	q    model.QuotaSnapshot
	err  error
}
type AntigravityCache struct {
	mu      sync.Mutex
	values  map[string]model.QuotaSnapshot
	pending map[string]*antiFlight
}

func NewAntigravityCache() *AntigravityCache {
	return &AntigravityCache{values: map[string]model.QuotaSnapshot{}, pending: map[string]*antiFlight{}}
}
func (c *AntigravityCache) read(ctx context.Context, key string, force bool, fetch func() (model.QuotaSnapshot, error)) (model.QuotaSnapshot, error) {
	c.mu.Lock()
	if f := c.pending[key]; f != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return unavailableAntigravity(), ctx.Err()
		case <-f.done:
			return f.q, f.err
		}
	}
	if q, ok := c.values[key]; ok && !force && time.Now().Before(q.ExpiresAt) {
		q.Cached = true
		c.mu.Unlock()
		return q, nil
	}
	f := &antiFlight{done: make(chan struct{})}
	c.pending[key] = f
	c.mu.Unlock()
	q, e := fetch()
	c.mu.Lock()
	defer c.mu.Unlock()
	if e != nil {
		if old, ok := c.values[key]; ok {
			q = old
			q.Stale = true
			q.Status = "stale"
			q.ErrorCode = fault.Code(e)
		}
	} else {
		if len(c.values) >= 8 {
			c.values = map[string]model.QuotaSnapshot{}
		}
		c.values[key] = q
	}
	f.q = q
	f.err = e
	delete(c.pending, key)
	close(f.done)
	return q, e
}

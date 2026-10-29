package quota

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAntigravityProcessDiscoveryMatchesLegacy(t *testing.T) {
	ps := []byte(" PID COMMAND\n10 /Applications/Editor.app/language_server --csrf_token=ide-token --app_data_dir antigravity-ide\n22 /Applications/Antigravity.app/Contents/language_server_macos --csrf_token standalone-token --standalone\n22 duplicate language_server --csrf_token bad-token\n90 /bin/unrelated --csrf_token not-relevant\n")
	processes := ParseAntigravityProcesses(ps)
	if len(processes) != 2 || processes[0].PID != 22 || processes[0].token != "standalone-token" || processes[1].token != "ide-token" {
		t.Fatal("incorrect process ranking")
	}
	if strings.Contains(fmt.Sprintf("%v", processes[0]), "standalone-token") {
		t.Fatal("token leaked in formatting")
	}
}
func TestAntigravityKeepsGroupsModelsAndDisabledMetadata(t *testing.T) {
	q, e := ParseAntigravity([]byte(`{"userStatus":{"cascadeModelConfigData":{"clientModelConfigs":[{"label":"m","quotaInfo":{"remainingFraction":0.25}}]}}}`), []byte(`{"response":{"description":"Plan","groups":[{"displayName":"Premium","description":"group info","buckets":[{"bucketId":"week","displayName":"Weekly","window":"weekly","remainingFraction":0.5},{"bucketId":"disabled","displayName":"Unavailable","disabled":true}]}]}}`), time.Now(), time.Minute)
	if e != nil || len(q.Models) != 1 || len(q.Groups) != 1 || len(q.Groups[0].Buckets) != 2 || !q.Groups[0].Buckets[1].Disabled || q.Description != "Plan" {
		t.Fatal(q, e)
	}
	if q.Models[0].RemainingFraction == nil || *q.Models[0].RemainingFraction != .25 {
		t.Fatal(q.Models)
	}
}
func TestAntigravityConfiguredPortAndAutomaticTokenWithLocalTLS(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Codeium-Csrf-Token") != "test-process-token" {
			t.Error("discovery token missing")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "GetUserStatus") {
			fmt.Fprint(w, `{"cascadeModelConfigData":{"clientModelConfigs":[{"label":"m","quotaInfo":{"remainingFraction":0.8}}]}}`)
		} else {
			fmt.Fprint(w, `{"response":{"groups":[]}}`)
		}
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	a := Antigravity{Port: port, Cache: NewAntigravityCache(), ScanProcesses: func(context.Context) ([]byte, error) {
		return []byte("42 /Applications/Antigravity.app/language_server --csrf_token test-process-token"), nil
	}}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, e := a.Read(context.Background())
			if e != nil || len(q.Models) != 1 || q.LocalPort != port {
				t.Errorf("read failed %v", e)
			}
		}()
	}
	wg.Wait()
	if requests.Load() != 2 {
		t.Fatalf("singleflight/cache failed: %d requests", requests.Load())
	}
}
func TestAntigravityMissingPortDoesNotScan(t *testing.T) {
	a := Antigravity{ScanProcesses: func(context.Context) ([]byte, error) {
		t.Fatal("must not scan ports or processes without configured endpoint")
		return nil, nil
	}}
	if _, e := a.Read(context.Background()); e == nil {
		t.Fatal("missing endpoint accepted")
	}
}

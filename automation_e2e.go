//go:build e2e

package main

// MyGo's official window automation can capture actual native screenshots.
// Only test builds link this plugin; production builds do not expose its
// automation socket. Start an E2E build with MYGO_AUTOMATION pointing at a
// private Unix socket to exercise and capture the real desktop windows.
import _ "github.com/egoist/mygo/plugins/automation"

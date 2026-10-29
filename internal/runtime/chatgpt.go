package runtime

import (
	"context"
	"fmt"
	"lam.local/lam2/internal/fault"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"
)

func ChatGPTProcessIDs(raw string, uid int) []int {
	ids := []int{}
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, e := strconv.Atoi(fields[0])
		if e != nil || pid <= 1 {
			continue
		}
		owner, e := strconv.Atoi(fields[1])
		if e != nil || owner != uid {
			continue
		}
		path := strings.Join(fields[2:], " ")
		if strings.HasPrefix(path, "/Applications/ChatGPT.app/Contents/") {
			ids = append(ids, pid)
		}
	}
	return ids
}
func ParseWindowBounds(raw string) (out [4]int, ok bool) {
	fields := strings.FieldsFunc(strings.TrimSpace(raw), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	if len(fields) != 4 {
		return out, false
	}
	for i, v := range fields {
		n, e := strconv.Atoi(v)
		if e != nil || n < -100000 || n > 100000 {
			return out, false
		}
		out[i] = n
	}
	return out, out[2] > 0 && out[3] > 0
}

const chatGPTBoundsScript = `tell application "System Events"
if exists process "ChatGPT" then
 tell process "ChatGPT"
  if exists window 1 then return (position of window 1 as list) & (size of window 1 as list)
 end tell
end if
end tell`

// RestartChatGPT is an explicit user action. It never matches a process by its
// display name alone: UID and the installed application bundle path must match.
func RestartChatGPT(ctx context.Context) error {
	if goruntime.GOOS != "darwin" {
		return fault.New("RESTART_CHATGPT_UNSUPPORTED", "ChatGPT restart is available on macOS")
	}
	if st, e := os.Stat("/Applications/ChatGPT.app"); e != nil || !st.IsDir() {
		return fault.New("CHATGPT_NOT_FOUND", "ChatGPT.app is not installed in /Applications")
	}
	call, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	bounds, _ := exec.CommandContext(call, "/usr/bin/osascript", "-e", chatGPTBoundsScript).Output()
	_ = exec.CommandContext(call, "/usr/bin/osascript", "-e", `tell application "System Events"
if exists process "ChatGPT" then tell application "ChatGPT" to quit
end tell`).Run()
	scan := func() ([]int, error) {
		out, e := exec.CommandContext(call, "/bin/ps", "-axo", "pid=,uid=,comm=").Output()
		if e != nil {
			return nil, e
		}
		return ChatGPTProcessIDs(string(out), os.Getuid()), nil
	}
	wait := func() error {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			ids, e := scan()
			if e != nil {
				return e
			}
			if len(ids) == 0 {
				return nil
			}
			select {
			case <-call.Done():
				return call.Err()
			case <-timer.C:
				return fault.New("CHATGPT_STILL_RUNNING", "ChatGPT is still running")
			case <-tick.C:
			}
		}
	}
	stopped := wait() == nil
	for _, signal := range []string{"-TERM", "-KILL"} {
		if stopped {
			break
		}
		ids, e := scan()
		if e != nil {
			return fault.Wrap("STOP_CHATGPT_FAILED", "Cannot inspect ChatGPT processes", e)
		}
		if len(ids) > 0 {
			args := []string{signal}
			for _, pid := range ids {
				args = append(args, strconv.Itoa(pid))
			}
			_ = exec.CommandContext(call, "/bin/kill", args...).Run()
		}
		stopped = wait() == nil
	}
	if !stopped {
		return fault.New("STOP_CHATGPT_FAILED", "ChatGPT did not exit; no replacement instance was started")
	}
	if e := exec.CommandContext(call, "/usr/bin/open", "/Applications/ChatGPT.app").Run(); e != nil {
		return fault.Wrap("RESTART_CHATGPT_FAILED", "Could not reopen ChatGPT", e)
	}
	if b, ok := ParseWindowBounds(string(bounds)); ok {
		script := fmt.Sprintf(`tell application "System Events"
repeat 20 times
 if exists process "ChatGPT" then
  tell process "ChatGPT"
   if exists window 1 then
    set position of window 1 to {%d, %d}
    set size of window 1 to {%d, %d}
    return
   end if
  end tell
 end if
 delay 0.1
end repeat
end tell`, b[0], b[1], b[2], b[3])
		_ = exec.CommandContext(call, "/usr/bin/osascript", "-e", script).Run()
	}
	return nil
}

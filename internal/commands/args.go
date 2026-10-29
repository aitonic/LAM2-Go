// Package commands parses a small native application's CLI; business operations
// remain in their services. Credentials are accepted from files/stdin, never flags.
package commands

import (
	"flag"
	"io"
	"lam.local/lam2/internal/fault"
	"strings"
)

type Invocation struct {
	Home, Mode, Action, Profile, ID, Provider, Model, Name, AuthMode, Path, Input, Output, SecretFile, Target, Policy, Fingerprint, Resume, CWD, Search, Sort, After, Before, Workspace, Listen, Kind, Cursor string
	Revision                                                                                                                                                                                                  uint64
	Offset, Limit                                                                                                                                                                                             int
	Confirm, AllowProviderChange                                                                                                                                                                              bool
	Extra                                                                                                                                                                                                     []string
}

func Parse(args []string) (Invocation, error) {
	v := Invocation{Mode: "desktop", Limit: 50, Sort: "modified", Policy: "stop", AuthMode: "profile", Listen: "127.0.0.1:0", Kind: "chatgpt", Input: "-", Output: "-"}
	bad := func() (Invocation, error) {
		return v, fault.New("ARGUMENT_INVALID", "Invalid command or flag; run lam help")
	}
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		v.Mode = "help"
		return v, nil
	}
	if len(args) > 0 && args[0] == "--home" {
		if len(args) < 2 {
			return bad()
		}
		v.Home = args[1]
		args = args[2:]
	}
	if len(args) > 0 {
		v.Mode = args[0]
		args = args[1:]
	}
	actions := map[string]string{"profile": "list discover create plan-relay create-relay register edit delete login read import export activate restore-pat repair-wrapper threads", "provider": "list save delete test discover plan-bind bind plan-detach detach approve-command", "session": "list read plan-delete archive restore", "handoff": "plan execute latest", "quota": "read refresh account-usage reset-plan reset-consume antigravity", "usage": "index summary calls threads daily reset compact reprice", "settings": "read save prices", "migrate": "plan apply rollback"}
	if list, ok := actions[v.Mode]; ok {
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return bad()
		}
		v.Action = args[0]
		args = args[1:]
		valid := false
		for _, s := range strings.Fields(list) {
			if s == v.Action {
				valid = true
			}
		}
		if !valid {
			return bad()
		}
	} else {
		switch v.Mode {
		case "desktop", "auth", "gateway", "launch", "doctor", "help":
		default:
			return bad()
		}
	}
	f := flag.NewFlagSet("lam", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&v.Home, "home", v.Home, "")
	f.StringVar(&v.Profile, "profile", "", "")
	f.StringVar(&v.ID, "id", "", "")
	f.StringVar(&v.ID, "binding", "", "")
	f.StringVar(&v.Provider, "provider", "", "")
	f.StringVar(&v.Model, "model", "", "")
	f.StringVar(&v.Name, "name", "", "")
	f.StringVar(&v.AuthMode, "mode", v.AuthMode, "")
	f.StringVar(&v.Path, "path", "", "")
	f.StringVar(&v.Input, "input", "-", "")
	f.StringVar(&v.Output, "output", "-", "")
	f.StringVar(&v.SecretFile, "secret-file", "", "")
	f.StringVar(&v.Target, "target", "", "")
	f.StringVar(&v.Target, "copy-config-from", "", "")
	f.StringVar(&v.Policy, "policy", v.Policy, "")
	f.StringVar(&v.Fingerprint, "fingerprint", "", "")
	f.StringVar(&v.Resume, "resume", "", "")
	f.StringVar(&v.CWD, "cwd", "", "")
	f.StringVar(&v.Search, "search", "", "")
	f.StringVar(&v.Sort, "sort", v.Sort, "")
	f.StringVar(&v.After, "after", "", "")
	f.StringVar(&v.Before, "before", "", "")
	f.StringVar(&v.Workspace, "workspace", "", "")
	f.StringVar(&v.Listen, "listen", v.Listen, "")
	f.StringVar(&v.Kind, "kind", v.Kind, "")
	f.StringVar(&v.Cursor, "cursor", "", "")
	f.StringVar(&v.Action, "action", v.Action, "")
	f.Uint64Var(&v.Revision, "revision", 0, "")
	f.IntVar(&v.Offset, "offset", 0, "")
	f.IntVar(&v.Limit, "limit", 50, "")
	f.BoolVar(&v.Confirm, "confirm", false, "")
	f.BoolVar(&v.AllowProviderChange, "allow-provider-change", false, "")
	if e := f.Parse(args); e != nil {
		return bad()
	}
	v.Extra = f.Args()
	if v.Mode != "launch" && len(v.Extra) > 0 {
		return bad()
	}
	if v.Offset < 0 || v.Limit < 1 || v.Limit > 1000 {
		return bad()
	}
	return v, nil
}

const Help = `LAM 2 - MyGo Native UI + Go services

Usage: lam [--home PRIVATE_DIRECTORY] [desktop | COMMAND]

  desktop                         Native MyGo desktop (default)
  doctor                          Read-only runtime diagnostics
  auth --binding ID               Output exactly one binding token
  gateway [--listen 127.0.0.1:0]   Run authenticated local gateway
  gateway --action status|stop    Verify identity before inspecting/stopping
  launch --profile ID [--resume SESSION] [--cwd PATH] [-- ARGS...]
  profile list|discover|create|register|edit|delete|login|read|import|export
          activate|restore-pat|repair-wrapper|threads|plan-relay|create-relay
  provider list|save|delete|test|discover|plan-bind|bind|plan-detach|detach|approve-command
  session list|read|plan-delete|archive|restore
  handoff plan|execute|latest
  quota read|refresh|account-usage|reset-plan|reset-consume|antigravity
  usage index|summary|calls|threads|daily|reset|compact|reprice
  settings read|save|prices
  migrate plan|apply|rollback

Shared flags: --profile ID --id ID --provider ID --model NAME --name NAME
 --input FILE|- --output FILE|- --secret-file FILE --revision N
 --target PROFILE --policy stop_and_ask|prefer_source|prefer_target|timeline_merge_to_fork|summarize_fork_with_target_account --fingerprint HASH --confirm
 --path DIRECTORY --after YYYY-MM-DD --before YYYY-MM-DD (exclusive)
 --offset N --limit N --workspace PATH --search TEXT

Writes with revisions use the value returned by settings read / doctor.
Sensitive imports/exports, PAT switches, archives and resets require --confirm.
Secrets are never accepted as literal command-line flags. See docs/CLI.md.
`

package codexsdk

import (
	"context"
	"errors"
	sdk "github.com/pmenglund/codex-sdk-go"
	"io"
	core "lam.local/lam2/internal/codex"
	"lam.local/lam2/internal/fault"
	"lam.local/lam2/internal/model"
	run "lam.local/lam2/internal/runtime"
	"os"
)

type Client struct{ client *sdk.Codex }

func Factory(path func() string, extra func(context.Context, model.Profile) (map[string]string, error)) core.Factory {
	return func(ctx context.Context, p model.Profile) (core.Client, error) {
		binary, err := run.ResolveExecutable(path())
		if err != nil {
			return nil, fault.Wrap("CODEX_NOT_FOUND", "Install the compatible Codex CLI or configure its path", err)
		}
		env, err := run.SDKOverlay(os.Environ(), p)
		if err != nil {
			return nil, err
		}
		if extra != nil {
			selected, e := extra(ctx, p)
			if e != nil {
				return nil, e
			}
			for k, v := range selected {
				if !run.EnvName(k) || k == "CODEX_HOME" {
					return nil, fault.New("INVALID_ENVIRONMENT", "Invalid selected provider environment")
				}
				env = append(env, k+"="+v)
			}
		}
		callbacks := sdk.RejectingApprovalCallbacks()
		c, err := sdk.New(ctx, sdk.Options{Spawn: sdk.SpawnOptions{CodexPath: binary, Env: env, Stderr: io.Discard}, RequestHandler: &callbacks})
		if err != nil {
			var mismatch *sdk.CodexCompatibilityError
			if errors.As(err, &mismatch) {
				return nil, fault.Wrap("CODEX_VERSION_UNSUPPORTED", "Codex CLI must match the pinned SDK compatibility policy", err)
			}
			return nil, fault.Wrap("CODEX_UNAVAILABLE", "Codex app-server could not start", err)
		}
		return &Client{client: c}, nil
	}
}
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if e := c.client.Client().Call(ctx, method, params, out); e != nil {
		return fault.Wrap("CODEX_RPC_FAILED", "Codex app-server operation failed", e)
	}
	return nil
}
func (c *Client) Close() error { return c.client.Close() }

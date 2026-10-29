# CLI reference

The binary must first pass the full native build. Commands here document implemented dispatch; they are not proof that the full binary compiled in the current container.

Run `lam help` for flags. Syntax is `lam [--home PRIVATE_DIRECTORY] MODE ACTION [FLAGS]`. The default is `desktop`. Shared flag parsing is exercised by offline tests.

| Mode | Actions |
|---|---|
| profile | list, discover, create, plan-relay, create-relay, register, edit, delete, login, read, import, export, activate, restore-pat, repair-wrapper, threads |
| provider | list, save, delete, test, discover, plan-bind, bind, plan-detach, detach, approve-command |
| session | list, read, plan-delete, archive, restore |
| handoff | plan, execute, latest |
| quota | read, refresh, account-usage, reset-plan, reset-consume, antigravity |
| usage | index, summary, calls, threads, daily, reset, compact, reprice |
| settings | read, save, prices |
| migrate | plan, apply, rollback |
| gateway | default serve; --action status or stop |
| auth | --binding ID; only the token goes to stdout |
| launch | --profile ID, optional --resume SESSION --cwd PATH; extra Codex arguments after -- |
| doctor | read-only diagnostics |

JSON inputs use `--input FILE` or stdin (`--input -`). Read a current entity before editing it so IDs/revisions and unmodified fields are preserved. Credentials use approved references or `--secret-file FILE`, never an API key literal in flags. Shell history must not contain tokens. The command-auth approval stores the executable path, arguments and digest.

Sensitive writes use `--confirm`; transactional operations use `--revision` and/or a plan ID and fingerprint. A changed source or revision invalidates a previous preview. Do not bypass this by manually editing state.json. The same-profile Handoff and symlink escape attempts are rejected.

Safe inspection in a dedicated test home, after a successful build:

```sh
lam --home "$HOME/.lam-next-test" doctor
lam --home "$HOME/.lam-next-test" profile list
lam --home "$HOME/.lam-next-test" session list --limit 25
lam --home "$HOME/.lam-next-test" provider list
lam --home "$HOME/.lam-next-test" settings read
```

Provider `test` invokes the original direct Responses models health check, not a generation call. `plan-detach --profile ID` returns a fingerprint; `detach --id PLAN --fingerprint HASH --confirm` validates it before restoring config.

Provider catalogs use original LAM operational defaults plus explicit `models[].codexCatalog` overrides. Tool shims preserve supported custom/namespace identities; unknown unrepresentable input still fails explicitly rather than silently dropping content. Actual external translator integration remains unrun.

`profile create --name NAME --mode profile --revision N --copy-config-from SOURCE_ID` optionally copies an unbound source config without authentication files. `profile plan-relay --input relay.json` accepts runtimeProfileId/sourceProfileId/name/providerPolicy; `profile create-relay --id PLAN --fingerprint HASH --confirm` creates an independent unauthenticated home. This is not bulk session Sync.

Handoff policies are `stop_and_ask`, `prefer_source`, `prefer_target`, `timeline_merge_to_fork`, and `summarize_fork_with_target_account`. Old development aliases `stop`, `replace`, and explicit new-ID `fork` remain accepted. Auxiliary forks/summaries preserve the original target; the native UI requests confirmation before target-account Resume.

Migration now includes legacy metadata, pending bindings and a real MyGo logical SQLite import implementation. The old writer must be stopped; source hashes/version are checked. Core/fake-reader and literal SQL tests passed, but the actual native SQLite/WAL path is not certified. Preserve the original ~/.lam and database until full acceptance.

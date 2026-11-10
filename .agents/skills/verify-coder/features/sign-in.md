# Sign in

A user signs in with email and password on `/login` and lands on the workspaces dashboard. A CLI user logs in with a session token.

## Sub-features

- `signin-password` signs in through the password form.
- `signin-redirect` sends a signed-in user from `/` to `/workspaces`.
- `signin-cli` logs the CLI in to the instance.

## How to get to it (user POV)

- Open `$VC_WEB_URL/login` in a browser.
- Open `$VC_WEB_URL/`, which redirects to `/login` when signed out.
- Run `coder login <url>` in a terminal.

## Driving it with the coder CLI and the API

Preconditions:

- `scripts/doctor.sh` passes.
- Password login is enabled (the default).

- **Web UI.** This skill drives no browser, so report `signin-password` and `signin-redirect` as not verified.
- **CLI.** `.coderv2/session` already holds the admin session that `develop.sh` wrote. Run `vc_coder whoami`. Its output contains `You're authenticated as admin`. Match a substring: without a TTY the line is wrapped in `[` and `]`. For a fresh login into a separate config folder, create a token with `tok="$(vc_coder tokens create --name <name> 2>/dev/null | tail -1)"`, then pipe it to the prompt: `printf '%s\n' "$tok" | env -u CODER_URL -u CODER_SESSION_TOKEN "$(vc_coder_bin)" --global-config "$dir" login "$VC_API_URL"`, with `dir="$(mktemp -d)"` outside the evidence folder, because it then holds a live session file. It prints `Welcome to Coder, admin! You're authenticated.` Afterwards run `vc_coder tokens remove <name>` and `rm -rf "$dir"`. Name the token `vc-login-<stamp>`. As evidence, save only the output of `env -u CODER_URL -u CODER_SESSION_TOKEN "$(vc_coder_bin)" --global-config "$dir" whoami`, never the folder itself. `vc_coder whoami` reads `.coderv2`, not the new folder. Never print the token or pass it as an argument. `coder login` refuses a token in `CODER_SESSION_TOKEN`, and `--token` puts it in the process list.

## Gotchas

- `coder login` refuses to run while `CODER_SESSION_TOKEN` is set, and a token passed as an argument shows in the process list. Unset `CODER_URL` and `CODER_SESSION_TOKEN`, and pipe the token to the prompt, as the CLI recipe does.

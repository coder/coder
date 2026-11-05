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

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes.
- Password login is enabled (the default).

- **Sign in.** Run `vc_browser_login`. It clears cookies, opens `/login`, fills the `Email` and `Password` labels, clicks the `Sign In` button, and waits for the path `/workspaces` and the title `Workspaces - Coder`.
- **Sign in as the member.** Run `vc_browser_login member@coder.com "$VC_ADMIN_PASSWORD"`. The navbar shows no `Admin settings` menu.
- **Wrong password.** Fill a wrong password and click `Sign In`. The URL stays on `/login` and an alert heading reads `Incorrect email or password.`
- **CLI.** `.coderv2/session` already holds the admin session that `develop.sh` wrote. Run `scripts/coder.sh whoami`. It prints `You're authenticated as admin`. For a fresh login into a separate config folder, create a token with `tok="$(scripts/coder.sh tokens create --name <name> 2>/dev/null | tail -1)"`, then pipe it to the prompt: `printf '%s\n' "$tok" | env -u CODER_URL -u CODER_SESSION_TOKEN "$(vc_coder_bin)" --global-config <dir> login "$VC_API_URL"`. It prints `Welcome to Coder, admin! You're authenticated.` Never print the token or pass it as an argument. `coder login` refuses a token in `CODER_SESSION_TOKEN`, and `--token` puts it in the process list.
- **Proof.** After sign-in, `vc_ab screenshot "$VC_EVIDENCE/<stamp>-signin-password/01-ui-workspaces.png"` and `vc_ab get title`.

## Gotchas

- The login page renders `<title>Sign in to Coder</title>`, but `document.title` read empty in agent-browser. Wait for the `Sign In` button, not the title.
- On the Vite URL (`$VC_VITE_URL`), after `cookies clear`, opening a cached `/login` leaves a CSRF cookie that does not match the dev site's fixed CSRF header, and the next POST fails with `Something is wrong with your CSRF token`. The UI then shows `Request failed with status code 400`. `vc_browser_login` adds a unique query to `/login` to avoid this. Do the same in any custom sign-in.
- A successful sign-in both renders a client-side redirect and sets `location.href` for a hard reload (`site/src/pages/LoginPage/LoginPage.tsx`). The workspaces page can render before the reload replaces it. `vc_browser_login` sets a marker on `window` before it clicks `Sign In` and waits until the marker is gone and the title is `Workspaces - Coder`.
- Inside a Coder workspace the dev build names the session cookie `dev_coder_session_token`. Sign in through the form instead of setting cookies.

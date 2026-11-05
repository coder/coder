# Users

An owner adds a user who signs in with a password, from the admin users page or the CLI. The new user appears in the users list and can sign in.

## Sub-features

- `users-create-ui` creates a password user on `/deployment/users/create`.
- `users-create-cli` creates a password user with `coder users create`.
- `users-roles` assigns roles from the row menu.
- `users-login` proves the new user can sign in.

## How to get to it (user POV)

- `Admin settings` > `Users`, or `$VC_WEB_URL/deployment/users`, then the `New user` link.
- Run `coder users create` in a terminal.

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes and the browser is signed in as the admin (`vc_browser_login`).

- **Whole journey.** Run `journeys/create-user.sh`. It ends with `create-user: 7/7 checks passed` and writes evidence to `build/verify-coder/evidence/<stamp>-create-user/`.
- **Open the form.** `vc_ab open "$VC_WEB_URL/deployment/users"`, wait for the title with `vc_ab wait --fn 'document.title === "Users - Coder"'`, screenshot the list as the before state, then `vc_ab find role link click --name "New user"`. The title becomes `New user - Coder`.
- **Fill the form.** The `Login type` select (`[data-testid=login-type-input]`) defaults to `Password`. Fill `find label "Username"`, `input[name=name]`, `find label "Email"` and `input[name=password]`. Screenshot before you type the password.
- **Save.** `vc_ab find role button click --name "Save"`. A toast reads `User "<username>" created successfully.` and the list shows the email. A new row can sit below the fold and under the toast. For the after screenshot, open `$VC_WEB_URL/deployment/users?filter=<username>` (the search box keeps its query there).
- **Second view.** `vc_api "/api/v2/users?q=<username>"` returns one user with `login_type` `password` and status `dormant`. `scripts/coder.sh users list --output json` includes the username.
- **Sign in as the user.** `POST $VC_API_URL/api/v2/users/login` with the email and password answers `201`. Do not print the returned token.
- **CLI create.** `scripts/coder.sh users create --username <u> --email <u>@coder.com --full-name "<name>" --password <pw> >/dev/null`. Prove it with the same API read and a UI load of `/deployment/users?filter=<u>`.
- **Roles.** On the filtered list, `vc_ab click 'tbody tr button[aria-label="Open menu"]'`, then `vc_ab find role menuitem click --name "Edit roles"`, `vc_ab find role checkbox click --name "Template Admin"` and `vc_ab find role button click --name "Confirm"`. The row shows `Template Admin`, and the API lists the role `template-admin`.

## Gotchas

- `coder users create` prints `Your password is: <password>`. Discard its stdout or redact it before you save it as evidence.
- `site/e2e/helpers.ts` uses `getByLabel("Login Type")`. The label is `Login type`. Playwright ignores case, agent-browser exact matches do not.
- `find label "Name"` also matches `Username`. Use `input[name=name]`.
- New users show `Dormant` until their first sign-in. That is not a failure.
- The `Organization` picker appears only with more than one organization or the multi-organization entitlement. The seed has one organization, so the form has no picker.

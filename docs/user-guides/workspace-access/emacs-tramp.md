# Emacs TRAMP

[Emacs TRAMP](https://www.emacswiki.org/emacs/TrampMode) is a method of running
editing operations on a remote server.

## Connecting To A Workspace

To connect to your workspace first run:

```shell
coder config-ssh
```

Then you can connect to your workspace by its name in the format:
`coder.<WORKSPACE NAME>`.

In Emacs type `C-x d` and then input: `/-:coder.<WORKSPACE NAME>:` and hit
enter. This will open up Dired on the workspace's home directory.

### Using SSH

By default Emacs TRAMP is setup to use SCP to access files on the Coder
workspace instance. However you might want to use SSH if you have a jumpbox or
some other complex network setup.

To do so set the following in your Emacs `init.el` file:

```lisp
(setq tramp-default-method "ssh")
```

Then when you access the workspace instance via `/-:coder.<WORKSPACE NAME>`
Emacs will use SSH. Setting `tramp-default-method` will also tell `ansi-term`
mode the correct way to access the remote when directory tracking.

## Directory Tracking

### ansi-term

If you run your terminal in Emacs via `ansi-term` then you might run into a
problem where while SSH-ed into a workspace Emacs will not change its
`default-directory` to open files in the directory your shell is in.

To fix this:

1. In your workspace Terraform template be sure to add the following:

   ```tf
   data "coder_workspace" "me" {
   }

   resource "coder_agent" "main" {
     # ...
     env = {
       name = "CODER_WORKSPACE_NAME"
       value = data.coder_workspace.me.name
     }
   }
   ```

2. Next in the shell profile file on the workspace (ex., `~/.bashrc` for Bash
   and `~/.zshrc` for Zsh) add the following:

   ```bash
   ansi_term_announce_host() {
       printf '\033AnSiTh %s\n' "coder.$CODER_WORKSPACE_NAME"
   }

   ansi_term_announce_user() {
       printf '\033AnSiTu %s\n' "$USER"
   }

   ansi_term_announce_pwd() {
       printf '\033AnSiTc %s\n' "$PWD"
   }

   ansi_term_announce() {
       ansi_term_announce_host
       ansi_term_announce_user
       ansi_term_announce_pwd
   }

   cd()    { command cd    "$@"; ansi_term_announce_pwd; }
   pushd() { command pushd "$@"; ansi_term_announce_pwd; }
   popd()  { command popd  "$@"; ansi_term_announce_pwd; }

   ansi_term_announce
   ```

   Ansi Term expects the terminal running inside of it to send escape codes to
   inform Emacs of the hostname, user, and working directory. The above code
   sends these escape codes and associated data whenever the terminal logs in
   and whenever the directory changes.

### eshell

The `eshell` mode will perform directory tracking by default, no additional
configuration is needed.

## Language Servers (Code Completion)

If you use [`lsp-mode`](https://emacs-lsp.github.io/lsp-mode) for code
intelligence and completion some additional configuration is required.

In your Emacs `init.el` file you must register a LSP client and tell `lsp-mode`
how to find it on the remote machine using the `lsp-register-client` function.
For each LSP server you want to use in your workspace add the following:

```lisp
(lsp-register-client (make-lsp-client :new-connection (lsp-tramp-connection "<LSP SERVER BINARY>")
              :major-modes '(<LANGUAGE MODE>)
              :remote? t
              :server-id '<LANGUAGE SERVER ID>))
```

This tells `lsp-mode` to look for a language server binary named
`<LSP SERVER BINARY>` for use in `<LANGUAGE MODE>` on a machine named
`coder.<WORKSPACE NAME>`. Be sure to replace the values between angle brackets:

- `<LSP SERVER BINARY>` : The name of the language server binary, without any
  path components. For example to use the Deno Javascript language server use
  the value `deno`.
- `<LANGUAGE MODE>`: The name of the Emacs major mode for which the language
  server should be used. For example to enable the language server for
  Javascript development use the value `web-mode`.
- `<LANGUAGE SERVER ID>`: This is just the name that `lsp-mode` will use to
  refer to this language server. If you are ever looking for output buffers or
  files they may have this name in them.

Calling the `lsp-register-client` function will tell `lsp-mode` the name of the
LSP server binary. However this binary must be accessible via the path. If the
language server binary is not in the path you must modify `tramp-remote-path` so
that `lsp-mode` knows in what directories to look for the LSP server. To do this
use TRAMP's connection profiles functionality. These connection profiles let you
customize variables depending on what machine you are connected to. Add the
following to your `init.el`:

```lisp
(connection-local-set-profile-variables 'remote-path-lsp-servers
                                 '((tramp-remote-path . ("<PATH TO ADD>" tramp-default-remote-path))))
(connection-local-set-profiles '(:machine "coder.<WORKSPACE NAME>") 'remote-path-lsp-servers)
```

The `connection-local-set-profile-variables` function creates a new connection
profile by the name `remote-path-lsp-servers`. The
`connection-local-set-profiles` then indicates this `remote-path-lsp-servers`
connection profile should be used when connecting to a server named
`coder.<WORKSPACE NAME>`. Be sure to replace `<PATH TO ADD>` with the directory
in which a LSP server is present.

TRAMP and `lsp-mode` are fickle friends, sometimes there is weird behavior. If
you find that language servers are hanging in the `starting` state then
[it might be helpful](https://github.com/emacs-lsp/lsp-mode/issues/2709#issuecomment-800868919)
to set the `lsp-log-io` variable to `t`.

More details on configuring `lsp-mode` for TRAMP can be found
[in the `lsp-mode` documentation](https://emacs-lsp.github.io/lsp-mode/page/remote/).
The
[TRAMP `tramp-remote-path` documentation](https://www.gnu.org/software/emacs/manual/html_node/tramp/Remote-programs.html#Remote-programs)
contains more examples and details of connection profiles.

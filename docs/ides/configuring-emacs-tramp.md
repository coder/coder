# Configuring Emacs TRAMP
[Emacs TRAMP](https://www.emacswiki.org/emacs/TrampMode) is a method of running editing operations on a remote server.

## Connecting To A Workspace
To connect to your workspace first run:

```
coder config-ssh
```

Then you can connect to your workspace by its name in the format: `coder.<name>`.

In Emacs type `C-x d` and then input: `/-:coder.<name>:` and hit enter. This will open up Dired on the workspace's home directory.

### Using SSH
By default Emacs TRAMP is setup to use SCP to access files on the Coder workspace instance. However you might want to use SSH if you have a jumpbox or some other complex network setup.

To do so set the following in your Emacs `init.el` file:

```lisp
(setq tramp-default-method "ssh")
```

Then when you access the workspace instance via `/-:coder.<name>` Emacs will use SSH. Setting `tramp-default-method` will also tell `ansi-term` mode the correct way to access the remote when directory tracking.

## Directory Tracking
### `ansi-term`
If you run your terminal in Emacs via `ansi-term` then you might run into a problem where while SSH-ed into a workspace Emacs will not change its `default-directory` to open files in the directory your shell is in.

To fix this:

1. In your workspace Terraform template be sure to add the following:
   ```hcl
   data "coder_workspace" "me" {
   }

   resource "coder_agent" "main" {
     # ...
     env {
       name = "CODER_WORKSPACE_NAME"
       value = data.coder_workspace.me.name
     }
   }
   ```
2. Next in the shell profile file on the workspace (ex., `~/.bashrc` for Bash and `~/.zshrc` for Zsh) add the following:
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
   Ansi Term expects the terminal running inside of it to send escape codes to inform Emacs of the hostname, user, and working directory. The above code sends these escape codes and associated data whenever the terminal logs in and whenever the directory changes.

### `eshell`
The `eshell` mode will perform directory tracking by default, no additional configuration is needed.

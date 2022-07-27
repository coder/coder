# Configuring Emacs TRAMP
[Emacs TRAMP](https://www.emacswiki.org/emacs/TrampMode) is a method of running editing operations on a remote server.

## Connecting To A Workspace
To connect to your workspace first run:

```
coder config-ssh
```

Then you can connect to your workspace by its name in the format: `coder.<name>`.

In Emacs type `C-x d` and then input: `/ssh:coder.<name>:` and hit enter. This will open up Dired on the workspace's home directory.

## Directory Tracking
### `ansi-term`
If you run your terminal in Emacs via `ansi-term` then you might run into a problem where while SSH-ed into a workspace Emacs will not change its `default-directory` to open files in the directory your shell is in.

To fix this:

1. In your Emacs `init.el` file add:
   ```lisp
   (setq tramp-default-method "ssh")
   ```
2. Then on your Coder workspace instance be sure to set the hostname to the `coder.<name>` format:
   ```bash
   hostname coder.<name>
   ```
   This can also be done in the workspace Terraform template by setting workspace instance's hostname to the data `coder_workspace.name` attribute. How this is done depends on how the instance is provisioned.
3. Next in the shell profile file on the workspace (ex., `~/.bashrc`) add the following:
   ```bash
   ansi_term_announce_host() {
       printf '\033AnSiTh %s\n' "$(hostname)"
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
   Ansi Term expects the terminal running inside of it to send escape codes to inform Emacs of the hostname, user, and working directory. The above code sends these escape codes and associated data whenever the terminal logs in and whenever the directory changes. The expression in step 1 lets Emacs know that you are accessing the hostname these escape codes announce via SSH.

# Dogfooding Guide

This guide explains how to
[dogfood](https://www.techopedia.com/definition/30784/dogfooding) coder for
employees at Coder.

## How to

The following explains how to do certain things related to dogfooding.

### Dogfood using Coder's Deployment

1. Go to
   [https://dev.coder.com/templates/coder-ts](https://dev.coder.com/templates/coder-ts)
   1. If you don't have an account, sign in with GitHub
   2. If you see a dialog/pop-up, hit "Cancel" (this is because of Rippling)
2. Create a workspace
3. [Connect with your favorite IDE](https://coder.com/docs/ides)
4. Clone the repo: `git clone git@github.com:coder/coder.git`
5. Follow the [contributing guide](https://coder.com/docs/CONTRIBUTING)

### Run Coder in your Coder Workspace

1. Clone the Git repo
    `[https://github.com/coder/coder](https://github.com/coder/coder)` and `cd`
    into it
2. Run `sudo apt update` and then `sudo apt install -y netcat`
    - skip this step if using the `coder` template
3. Run `make bin`

    <aside>
    💡 If you run into the following error:

    ```js
    pg_dump: server version: 13.7 (Debian 13.7-1.pgdg110+1); pg_dump version: 11.16 (Ubuntu 11.16-1.pgdg20.04+1)
    pg_dump: aborting because of server version mismatch
    ```

    Don’t fret! This is a known issue. To get around it:

    1. Add `export DB_FROM=coderdb` to your `.bashrc` (make sure you
       `source ~/.bashrc`)
    2. Run `sudo service postgresql start`
    3. Run `sudo -u postgres psql` (this will open the PostgreSQL CLI)
    4. Run `postgres-# alter user postgres password 'postgres';`
    5. Run `postgres-# CREATE DATABASE coderdb;`
    6. Run `postgres-# grant all privileges on database coderdb to postgres;`
    7. Run `exit` to exit the PostgreSQL terminal
    8. Try `make bin` again.
    </aside>

4. Run `./scripts/develop.sh` which will start _two_ separate processes:
    1. `[http://localhost:3000](http://localhost:3000)` — backend API server
       👈 Backend devs will want to talk to this
    2. `[http://localhost:8080](http://localhost:8080)` — Node.js dev server
       👈 Frontend devs will want to talk to this
5. Ensure that you’re logged in: `./scripts/coder-dev.sh list` — should return
    no workspace. If this returns an error, double-check the output of running
    `scripts/develop.sh`.
6. A template named `docker-amd64` (or `docker-arm64` if you’re on ARM) will
    have automatically been created for you. If you just want to create a
    workspace quickly, you can run
    `./scripts/coder-dev.sh create myworkspace -t docker-amd64` and this will
    get you going quickly!
7. To create your own template, you can do:
    `./scripts/coder-dev.sh templates init` and choose your preferred option.
    For example, choosing “Develop in Docker” will create a new folder `docker`
    that contains the bare bones for starting a Docker workspace template. Then,
    enter the folder that was just created and customize as you wish.

      <aside>
      💡 **For all Docker templates:**
      This step depends on whether you are developing on a Coder v1 workspace, versus a Coder v2 workspace, versus a VM, versus locally. In any case, check the output of the command `docker context ls` to determine where your Docker daemon is listening. Then open `./docker/main.tf` and check inside the block `provider "docker"` that the variable `"host"` is set correctly.
      </aside>

## Troubleshooting

### My Docker containers keep failing and I have no idea what's going on

```console
✔ Queued [236ms]
✔ Setting up [5ms]
⧗  Starting workspace
  Terraform 1.1.9
  coder_agent.dev: Plan to create
  docker_volume.home_volume: Plan to create
  docker_container.workspace[0]: Plan to create
  Plan: 3 to add, 0 to change, 0 to destroy.
  coder_agent.dev: Creating...
  coder_agent.dev: Creation complete after 0s [id=b2f132bd-9af1-48a7-81dc-187a18ee00d5]
  docker_volume.home_volume: Creating...
  docker_volume.home_volume: Creation complete after 0s [id=coder-maf-mywork-root]
  docker_container.workspace[0]: Creating...
  docker_container.workspace[0]: Creation errored after 0s
  Error: container exited immediately

✘ Starting workspace [2045ms]
terraform apply: exit status 1
Run 'coder create --help' for usage.
```

Check the output of `docker ps -a`

- If you see a container with the status `Exited` run
  `docker logs <container name>` and see what the issue with the container
  output is

Enable verbose container logging for Docker:

```shell
sudo cp /etc/docker/daemon.json /etc/docker/daemon.json.orig
sudo cat > /etc/docker/daemon.json << EOF
{
        "debug": true,
        "log-driver": "journald"
}
EOF
sudo systemctl restart docker
# You should now see container logs in journald.
# Try starting a workspace again and see what the actual error is!
sudo journalctl -u docker -f
```

### Help! I'm still blocked

Post in the #dogfood Slack channel internally or open a Discussion on GitHub and
tag @jsjoeio or @bpmct

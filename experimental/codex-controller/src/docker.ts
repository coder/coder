// Docker boundary for the OpenAI webhook prototype.
//
// Alternative to the bubblewrap path in coder.ts for templates where user
// namespaces are unavailable (bwrap fails writing uid_map). Selected
// explicitly by callers; coder.ts is unchanged.
//
// The executor runs in one Docker container per workspace, created through the
// workspace's own Docker daemon. The daemon may host unrelated containers, so
// the helper only ever lists containers carrying the prototype label and only
// stops or removes a container whose labels match this workspace and session.
//
// Container boundary: image pinned by immutable ID (Dockerfile.executor:
// Debian bookworm, python3, ca-certificates, and an ENTRYPOINT of
// executor-entrypoint.py), user 1000:1000, read-only root, all capabilities
// dropped, no-new-privileges, bridge network, no Docker socket, a bounded
// tmpfs /tmp (HOME and CODEX_HOME live there), the demo directory as the only
// writable bind, and the codex binary bind mounted read-only at
// /usr/local/bin/codex.
//
// Secret handling: the executor key travels only over `coder ssh` stdin to the
// fixed helper, then over `docker start --attach --interactive` stdin as one
// JSON line ({remote_url, environment_id, executor_key}) to the image
// entrypoint, which execs codex with the key in its environment. It never
// appears in CLI args, container config (`docker inspect`), labels, files, or
// logs. Container logging is disabled; the supervisor keeps a redacted log.

import { createHash } from "node:crypto";
import {
	CoderExecutorError,
	type CoderRunner,
	type ConnectInput,
	defaultRunner,
	type ExecutorStatus,
	type HelperResult,
	type Log,
	parseHelperOutput,
	redact,
	validateExecutorKey,
	validateId,
	validateRemoteUrl,
	validateWorkspace,
} from "./coder.ts";

/**
 * Immutable local image ID of codex-webhook-executor:stage2 built from
 * Dockerfile.executor. Rebuilding the image requires updating this constant.
 */
export const DOCKER_IMAGE =
	"sha256:0216facb0a29a438281b312bc5807c03a4d6281b9be893713350b9de93df0fa0";

export interface DockerDeps {
	runner?: CoderRunner;
	/** Overrides DOCKER_IMAGE; still validated. */
	image?: string;
}

export interface DockerExecutorStatus extends ExecutorStatus {
	containerId?: string;
}

const CONNECT_TIMEOUT_MS = 180_000;
const STOP_TIMEOUT_MS = 90_000;
const STATUS_TIMEOUT_MS = 45_000;

const IMAGE_PATTERN =
	/^(?:[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_][A-Za-z0-9._-]{0,127})?@)?sha256:[0-9a-f]{64}$/;

/** Accepts only digest-pinned image references. */
export function validateDockerImage(image: unknown): string {
	if (typeof image !== "string" || !IMAGE_PATTERN.test(image))
		throw new Error("docker image must be pinned by sha256 digest");
	return image;
}

export const DOCKER_LABEL_PREFIX = "dev.coder.codex-stage2.";
export const DOCKER_PROTOTYPE = "docker-executor-v1";

/** Deterministic container name for a session. Mirrors `container_name` in the helper. */
export function dockerContainerName(sessionId: string): string {
	return (
		"codex-stage2-" +
		createHash("sha256")
			.update(validateId("session id", sessionId))
			.digest("hex")
			.slice(0, 24)
	);
}

/** Labels that mark a container as owned by this workspace and session. Mirrors `labels_for` in the helper. */
export function dockerLabels(
	workspace: string,
	sessionId: string,
	environmentId: string,
	remoteUrl: string,
): Record<string, string> {
	return {
		[`${DOCKER_LABEL_PREFIX}prototype`]: DOCKER_PROTOTYPE,
		[`${DOCKER_LABEL_PREFIX}owner`]: validateWorkspace(workspace),
		[`${DOCKER_LABEL_PREFIX}session`]: validateId("session id", sessionId),
		[`${DOCKER_LABEL_PREFIX}environment`]: validateId(
			"environment id",
			environmentId,
		),
		[`${DOCKER_LABEL_PREFIX}remote-url`]: validateRemoteUrl(remoteUrl),
	};
}

// Fixed helper run over `coder ssh`. It receives exactly one JSON line on stdin
// and prints result lines prefixed with "STAGE2 ". It must not contain
// user-controlled data.
const HELPER = String.raw`
import fcntl, hashlib, json, os, re, signal, subprocess, sys, time
from pathlib import Path

CONTROL = Path('/home/coder/.codex-stage2-docker')
STATE = CONTROL / 'executor.json'
LOCK = CONTROL / 'lock'
LOG = CONTROL / 'executor.log'
DOCKER = '/usr/bin/docker'
# Bind sources are paths as seen by the Docker daemon; targets are fixed by the image entrypoint.
BINARY = '/home/coder/codex-cli/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex'
DEMO = '/home/coder/demo'
CONTAINER_BINARY = '/usr/local/bin/codex'
CONTAINER_DEMO = '/home/coder/demo'
ENTRYPOINT = ['python3', '/usr/local/bin/executor-entrypoint.py']
LABEL = 'dev.coder.codex-stage2.'
PROTOTYPE = 'docker-executor-v1'
READY_MARK = b'Noise executor connected to rendezvous'
READY_TIMEOUT = 90
STOP_TIMEOUT = 10
LOCK_TIMEOUT = 30
DOCKER_TIMEOUT = 30
ID_RE = re.compile(r'^[A-Za-z0-9_-]{1,200}$')
URL_RE = re.compile(r'^https://api\.openai\.com/v1/agents/api/connect(?:/[A-Za-z0-9_-]{1,200}){1,8}$')
WS_RE = re.compile(r'^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*/[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$')
IMAGE_RE = re.compile(r'^(?:[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[A-Za-z0-9_][A-Za-z0-9._-]{0,127})?@)?sha256:[0-9a-f]{64}$')
CID_RE = re.compile(r'^[0-9a-f]{64}$')
ANSI_RE = re.compile(r'\x1b\[[0-9;]*[A-Za-z]')
ACTIVE = ('running', 'paused', 'restarting')
DOCKER_ENV_ALLOW = ('HOME', 'DOCKER_HOST', 'DOCKER_CONFIG', 'DOCKER_CONTEXT', 'DOCKER_CERT_PATH', 'DOCKER_TLS_VERIFY')

def emit(obj):
    sys.stdout.write('STAGE2 ' + json.dumps(obj, sort_keys=True) + '\n')
    sys.stdout.flush()

def fail(code, message, **extra):
    extra.update(ok=False, error=code, message=message)
    emit(extra)
    sys.exit(3)

def proc_start(pid):
    try:
        data = Path('/proc/%d/stat' % pid).read_bytes()
    except OSError:
        return None
    rest = data[data.rindex(b')') + 2:].split()
    return None if rest[0] in (b'Z', b'X') else int(rest[19])

def alive(pid, start):
    return isinstance(pid, int) and pid > 1 and start is not None and proc_start(pid) == start

def read_state():
    try:
        st = json.loads(STATE.read_text())
    except FileNotFoundError:
        return None
    except (OSError, ValueError):
        fail('state_corrupt', 'executor state file is unreadable')
    return st if isinstance(st, dict) else None

def write_state(st):
    tmp = CONTROL / ('executor.json.%d' % os.getpid())
    fd = os.open(str(tmp), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(st, f)
    os.replace(str(tmp), str(STATE))

def update(nonce, **fields):
    try:
        st = json.loads(STATE.read_text())
    except (OSError, ValueError):
        return False
    if not isinstance(st, dict) or st.get('nonce') != nonce:
        return False
    st.update(fields)
    write_state(st)
    return True

def log_tail():
    try:
        with open(str(LOG), 'rb') as f:
            f.seek(0, 2)
            f.seek(max(0, f.tell() - 6000))
            text = f.read().decode('utf-8', 'replace')
    except OSError:
        return []
    return [ANSI_RE.sub('', l)[:400] for l in text.splitlines()[-15:]]

def lock():
    CONTROL.mkdir(mode=0o700, exist_ok=True)
    os.chmod(str(CONTROL), 0o700)
    fd = os.open(str(LOCK), os.O_RDWR | os.O_CREAT, 0o600)
    deadline = time.time() + LOCK_TIMEOUT
    while True:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return fd
        except BlockingIOError:
            if time.time() > deadline:
                fail('lock_timeout', 'another executor operation is in progress')
            time.sleep(0.2)

def docker_env():
    env = {k: v for k, v in os.environ.items() if k in DOCKER_ENV_ALLOW}
    env['PATH'] = '/usr/local/bin:/usr/bin:/bin'
    return env

def docker(args, timeout=DOCKER_TIMEOUT):
    try:
        r = subprocess.run([DOCKER] + args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                           stderr=subprocess.PIPE, env=docker_env(), timeout=timeout)
    except subprocess.TimeoutExpired:
        fail('docker_timeout', 'docker %s timed out' % args[0])
    except OSError as e:
        fail('docker_unavailable', 'cannot run docker: ' + e.__class__.__name__)
    return r.returncode, r.stdout.decode('utf-8', 'replace'), r.stderr.decode('utf-8', 'replace')

def inspect(ref):
    rc, out, err = docker(['container', 'inspect', ref])
    if rc != 0:
        if 'No such' in err:
            return None
        fail('docker_failed', 'docker inspect failed: ' + err.strip()[:300])
    try:
        data = json.loads(out)
    except ValueError:
        fail('docker_failed', 'docker inspect returned invalid JSON')
    return data[0] if isinstance(data, list) and data and isinstance(data[0], dict) else None

def labels_of(c):
    return (c.get('Config') or {}).get('Labels') or {}

def is_active(c):
    return (c.get('State') or {}).get('Status') in ACTIVE

def is_ours(c, ws, sid):
    l = labels_of(c)
    return l.get(LABEL + 'prototype') == PROTOTYPE and l.get(LABEL + 'owner') == ws and l.get(LABEL + 'session') == sid

def container_name(sid):
    return 'codex-stage2-' + hashlib.sha256(sid.encode()).hexdigest()[:24]

def labels_for(cfg):
    return {LABEL + 'prototype': PROTOTYPE, LABEL + 'owner': cfg['workspace'],
            LABEL + 'session': cfg['session_id'], LABEL + 'environment': cfg['environment_id'],
            LABEL + 'remote-url': cfg['remote_url']}

def create_args(cfg, name):
    args = ['create', '--name', name]
    for k, v in sorted(labels_for(cfg).items()):
        args += ['--label', k + '=' + v]
    args += ['--pull', 'never', '--interactive', '--init', '--user', '1000:1000', '--read-only',
             '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges', '--network', 'bridge',
             '--ipc', 'private', '--pids-limit', '512', '--hostname', 'codex-executor',
             '--restart', 'no', '--stop-timeout', str(STOP_TIMEOUT), '--log-driver', 'none',
             '--tmpfs', '/tmp:rw,nosuid,nodev,size=512m,mode=1777',
             '--mount', 'type=bind,source=%s,target=%s,readonly' % (BINARY, CONTAINER_BINARY),
             '--mount', 'type=bind,source=%s,target=%s' % (DEMO, CONTAINER_DEMO),
             '--workdir', CONTAINER_DEMO, cfg['image']]
    return args

def prototype_containers():
    rc, out, err = docker(['ps', '--all', '--no-trunc', '--filter', 'label=%sprototype=%s' % (LABEL, PROTOTYPE),
                           '--format', '{{.ID}}'])
    if rc != 0:
        fail('docker_failed', 'docker ps failed: ' + err.strip()[:300])
    found = []
    for cid in out.split():
        c = inspect(cid) if CID_RE.match(cid) else None
        if c:
            found.append(c)
    return found

def kill_supervisor(st):
    """Kills the recorded supervisor and its docker CLI child (same process group)."""
    pid, start = st.get('supervisor_pid'), st.get('supervisor_start')
    if not alive(pid, start):
        return
    try:
        pg = os.getpgid(pid)
    except OSError:
        return
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            if pg != os.getpgrp():
                os.killpg(pg, sig)
            else:
                os.kill(pid, sig)
        except OSError:
            pass
        deadline = time.time() + 3
        while time.time() < deadline and alive(pid, start):
            time.sleep(0.1)
        if not alive(pid, start):
            return

def stop_container(cid):
    """Stops the exact container ID and returns True once it is verified not running."""
    docker(['stop', '--time', str(STOP_TIMEOUT), cid], timeout=STOP_TIMEOUT + DOCKER_TIMEOUT)
    c = inspect(cid)
    if c and is_active(c):
        docker(['kill', cid])
        deadline = time.time() + 10
        while time.time() < deadline:
            c = inspect(cid)
            if not c or not is_active(c):
                break
            time.sleep(0.25)
    return not c or not is_active(c)

def cleanup_run(st):
    kill_supervisor(st)
    cid = st.get('container_id')
    if not isinstance(cid, str) or not CID_RE.match(cid):
        return True
    c = inspect(cid)
    if not c:
        return True
    if not is_ours(c, st.get('workspace'), st.get('session_id')):
        return False
    return stop_container(cid)

def supervise(cfg, nonce, cid):
    signal.signal(signal.SIGHUP, signal.SIG_IGN)
    devnull = os.open('/dev/null', os.O_RDWR)
    for fd in (0, 1, 2):
        os.dup2(devnull, fd)
    os.closerange(3, 65536)
    os.umask(0o077)
    me = os.getpid()
    if not update(nonce, supervisor_pid=me, supervisor_start=proc_start(me), status='starting'):
        return
    keyb = cfg['executor_key'].encode()
    line = (json.dumps({'remote_url': cfg['remote_url'], 'environment_id': cfg['environment_id'],
                        'executor_key': cfg['executor_key']}) + '\n').encode()
    cfg = None
    logf = open(str(LOG), 'wb', buffering=0)
    try:
        proc = subprocess.Popen([DOCKER, 'start', '--attach', '--interactive', cid], stdin=subprocess.PIPE,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, env=docker_env(), close_fds=True)
    except OSError as e:
        update(nonce, status='exited', launch_error=e.__class__.__name__)
        return
    update(nonce, docker_cli_pid=proc.pid, docker_cli_start=proc_start(proc.pid))
    # stdin stays open: EOF handling differs across docker CLI versions and the
    # entrypoint reads exactly one line.
    try:
        proc.stdin.write(line)
        proc.stdin.flush()
    except OSError:
        pass
    line = None
    ready = False
    for chunk in proc.stdout:
        if keyb in chunk:
            chunk = chunk.replace(keyb, b'[REDACTED]')
        logf.write(chunk)
        if not ready and READY_MARK in chunk:
            ready = True
            update(nonce, status='ready', ready_at=time.time())
    rc = proc.wait()
    update(nonce, status='exited', exit_code=rc, exited_at=time.time())

def spawn(cfg, nonce, cid):
    pid = os.fork()
    if pid == 0:
        try:
            os.setsid()
            if os.fork() == 0:
                supervise(cfg, nonce, cid)
        except BaseException:
            pass
        os._exit(0)
    os.waitpid(pid, 0)

def wait_ready(nonce, reused):
    t0 = time.time()
    while time.time() - t0 < READY_TIMEOUT:
        st = read_state()
        if not st or st.get('nonce') != nonce:
            fail('state_changed', 'executor state changed unexpectedly')
        sup = alive(st.get('supervisor_pid'), st.get('supervisor_start'))
        if st.get('status') == 'ready':
            c = inspect(st['container_id'])
            if c and is_active(c) and sup:
                emit({'ok': True, 'status': 'ready', 'reused': reused, 'container_id': st['container_id']})
                return
        if st.get('status') in ('ready', 'exited') or (st.get('supervisor_pid') and not sup):
            stopped = cleanup_run(st)
            fail('executor_exited', 'executor exited before rendezvous', exit_code=st.get('exit_code'),
                 container_id=st['container_id'], stopped=stopped, log_tail=log_tail())
        if not st.get('supervisor_pid') and time.time() - t0 > 15:
            st['nonce'] = 'abandoned-' + nonce
            st['status'] = 'exited'
            write_state(st)
            stopped = cleanup_run(st)
            fail('supervisor_missing', 'detached supervisor did not start', container_id=st['container_id'], stopped=stopped)
        time.sleep(0.25)
    st = read_state() or {}
    stopped = cleanup_run(st)
    update(nonce, status='exited', timed_out=True)
    fail('ready_timeout', 'executor did not reach rendezvous in time', container_id=st.get('container_id'),
         stopped=stopped, log_tail=log_tail())

def check_ids(cfg, names):
    for n in names:
        if not isinstance(cfg.get(n), str) or not ID_RE.match(cfg[n]):
            fail('invalid_input', 'invalid ' + n)

def check_workspace(cfg):
    if not isinstance(cfg.get('workspace'), str) or not WS_RE.match(cfg['workspace']):
        fail('invalid_input', 'invalid workspace')

def preflight(cfg):
    if not os.access(BINARY, os.X_OK) or not os.path.isfile(BINARY):
        fail('not_prepared', 'workspace is missing codex binary ' + BINARY)
    if not os.path.isdir(DEMO):
        fail('not_prepared', 'workspace is missing ' + DEMO)
    rc, out, _ = docker(['image', 'inspect', '--format', '{{json .Config.Entrypoint}}', cfg['image']])
    if rc != 0:
        fail('not_prepared', 'pinned executor image is not present locally')
    try:
        entrypoint = json.loads(out)
    except ValueError:
        entrypoint = None
    if entrypoint != ENTRYPOINT:
        fail('not_prepared', 'pinned executor image has an unexpected entrypoint')

def connect(cfg):
    check_ids(cfg, ('session_id', 'environment_id'))
    check_workspace(cfg)
    if not isinstance(cfg.get('remote_url'), str) or not URL_RE.match(cfg['remote_url']):
        fail('invalid_input', 'untrusted remote URL')
    if not isinstance(cfg.get('image'), str) or not IMAGE_RE.match(cfg['image']):
        fail('invalid_input', 'image must be pinned by digest')
    key = cfg.get('executor_key')
    if not isinstance(key, str) or len(key) < 8 or re.search(r'[\s\x00-\x1f]', key):
        fail('invalid_input', 'invalid executor key')
    ws, sid = cfg['workspace'], cfg['session_id']
    lock()
    for c in prototype_containers():
        if is_active(c) and not is_ours(c, ws, sid):
            fail('busy', 'another prototype executor container is active', owner_session=labels_of(c).get(LABEL + 'session'))
    name = container_name(sid)
    c = inspect(name)
    if c and not is_ours(c, ws, sid):
        fail('name_conflict', 'container ' + name + ' exists without matching ownership labels')
    st = read_state()
    if c:
        cid = c['Id']
        conf = c.get('Config') or {}
        own = labels_for(cfg)
        if {k: labels_of(c).get(k) for k in own} != own:
            fail('conflict', 'existing container has a different environment for this session', container_id=cid)
        if conf.get('Image') != cfg['image']:
            fail('conflict', 'existing container uses a different image', container_id=cid)
        mine = bool(st) and st.get('container_id') == cid
        if is_active(c):
            sup = mine and alive(st.get('supervisor_pid'), st.get('supervisor_start'))
            if sup and st.get('status') == 'ready':
                emit({'ok': True, 'status': 'ready', 'reused': True, 'container_id': cid})
                return
            if sup and st.get('status') in ('launching', 'starting'):
                wait_ready(st['nonce'], True)
                return
            # Running without an observing supervisor: readiness is unknown, restart it.
            if mine:
                kill_supervisor(st)
            if not stop_container(cid):
                fail('cleanup_incomplete', 'unobserved container is still running', container_id=cid)
        preflight(cfg)
    else:
        preflight(cfg)
        rc, out, err = docker(create_args(cfg, name))
        cid = out.strip()
        if rc != 0 or not CID_RE.match(cid):
            fail('create_failed', 'docker create failed: ' + err.strip()[:300])
    nonce = os.urandom(16).hex()
    # Persisted immediately after create so the container is always traceable.
    write_state({'nonce': nonce, 'container_id': cid, 'name': name, 'workspace': ws, 'session_id': sid,
                 'environment_id': cfg['environment_id'], 'remote_url': cfg['remote_url'], 'image': cfg['image'],
                 'status': 'launching', 'created_at': time.time()})
    spawn(cfg, nonce, cid)
    cfg['executor_key'] = None
    wait_ready(nonce, False)

def stop(cfg):
    check_ids(cfg, ('session_id',))
    check_workspace(cfg)
    ws, sid = cfg['workspace'], cfg['session_id']
    lock()
    st = read_state()
    st_cid = st.get('container_id') if st else None
    st_cid = st_cid if isinstance(st_cid, str) and CID_RE.match(st_cid) else None
    mine = bool(st) and st.get('session_id') == sid and st.get('workspace') == ws
    c = inspect(st_cid if mine and st_cid else container_name(sid))
    if c is None:
        if st and not mine and st_cid:
            other = inspect(st_cid)
            if other and is_active(other):
                fail('not_owner', 'executor is owned by another session', owner_session=labels_of(other).get(LABEL + 'session'))
        if mine:
            kill_supervisor(st)
            if st.get('status') != 'stopped':
                st['status'] = 'stopped'
                write_state(st)
        emit({'ok': True, 'status': 'not_running'})
        return
    cid = c['Id']
    if not is_ours(c, ws, sid):
        fail('not_owner', 'container is not owned by this session', owner_session=labels_of(c).get(LABEL + 'session'))
    if mine:
        kill_supervisor(st)
    if not stop_container(cid):
        fail('cleanup_incomplete', 'container still running after stop', container_id=cid)
    rc, _, err = docker(['rm', cid])
    if rc != 0 or inspect(cid) is not None:
        fail('cleanup_incomplete', 'stopped container could not be removed: ' + err.strip()[:300], container_id=cid)
    if mine:
        st['status'] = 'stopped'
        st['stopped_at'] = time.time()
        write_state(st)
    emit({'ok': True, 'status': 'stopped', 'container_id': cid})

def status(cfg):
    check_workspace(cfg)
    st = read_state()
    cid = st.get('container_id') if st else None
    if not isinstance(cid, str) or not CID_RE.match(cid):
        emit({'ok': True, 'status': 'none', 'alive': False})
        return
    c = inspect(cid)
    if c and not is_ours(c, cfg['workspace'], st.get('session_id')):
        fail('not_owner', 'recorded container does not carry this executor ownership labels', container_id=cid)
    live = bool(c) and is_active(c)
    state = st.get('status')
    if live and state not in ('launching', 'starting', 'ready'):
        state = 'stale'
    if not live and state in ('launching', 'starting', 'ready'):
        state = 'stale'
    emit({'ok': True, 'status': state, 'alive': live, 'session_id': st.get('session_id'),
          'environment_id': st.get('environment_id'), 'container_id': cid})

def main():
    try:
        cfg = json.loads(sys.stdin.readline() or '{}')
    except ValueError:
        fail('invalid_input', 'stdin must be one JSON line')
    op = cfg.get('op') if isinstance(cfg, dict) else None
    ops = {'connect': connect, 'stop': stop, 'status': status}
    if op not in ops:
        fail('invalid_input', 'unknown op')
    ops[op](cfg)

main()
`;

/** Source of the fixed remote helper, exported for tests. */
export const DOCKER_REMOTE_HELPER_SOURCE = HELPER;

/** The exact, constant remote command passed after `--` to `coder ssh`. */
export const DOCKER_REMOTE_COMMAND = `python3 -c "import base64;exec(base64.b64decode('${Buffer.from(HELPER).toString("base64")}'))"`;

/** Builds the `coder ssh` argument array for a validated workspace. */
export function buildDockerSshArgs(workspace: string): string[] {
	return [
		"ssh",
		"--disable-autostart",
		validateWorkspace(workspace),
		"--",
		DOCKER_REMOTE_COMMAND,
	];
}

function redactDeep(value: unknown, secrets: readonly string[]): unknown {
	if (typeof value === "string") return redact(value, secrets);
	if (Array.isArray(value)) return value.map((v) => redactDeep(v, secrets));
	if (value && typeof value === "object") {
		return Object.fromEntries(
			Object.entries(value).map(([k, v]) => [k, redactDeep(v, secrets)]),
		);
	}
	return value;
}

async function callHelper(
	workspace: string,
	payload: Record<string, unknown>,
	timeoutMs: number,
	secrets: string[],
	deps: DockerDeps,
): Promise<HelperResult> {
	const runner = deps.runner ?? defaultRunner;
	const res = await runner(
		buildDockerSshArgs(workspace),
		`${JSON.stringify(payload)}\n`,
		timeoutMs,
	);
	const result = parseHelperOutput(res.stdout);
	if (!result) {
		const tail = redact(res.stderr, secrets)
			.split("\n")
			.filter(Boolean)
			.slice(-5)
			.join(" | ")
			.slice(0, 800);
		throw new CoderExecutorError(
			"ssh_failed",
			`coder ssh failed (exit ${res.code}): ${tail}`,
		);
	}
	return redactDeep(result, secrets) as HelperResult;
}

function helperError(result: HelperResult): CoderExecutorError {
	const { ok: _ok, error, message, ...details } = result;
	return new CoderExecutorError(
		String(error ?? "unknown"),
		String(message ?? "docker executor helper failed"),
		details,
	);
}

/**
 * Starts (or reattaches to) the workspace's Docker executor container for a
 * session. Resolves only after rendezvous. Idempotent for the same session;
 * rejects with `busy` if another session's container is active.
 */
export async function connectDockerWorkspace(
	input: ConnectInput,
	log: Log,
	deps: DockerDeps = {},
): Promise<void> {
	const workspace = validateWorkspace(input.workspace);
	const sessionId = validateId("session id", input.sessionId);
	const environmentId = validateId("environment id", input.environmentId);
	const remoteUrl = validateRemoteUrl(input.remoteUrl);
	const executorKey = validateExecutorKey(input.executorKey);
	const image = validateDockerImage(deps.image ?? DOCKER_IMAGE);
	const secrets = [executorKey];
	log("docker_executor_connect", {
		workspace,
		session: sessionId,
		environment: environmentId,
		image,
	});
	const result = await callHelper(
		workspace,
		{
			op: "connect",
			workspace,
			session_id: sessionId,
			environment_id: environmentId,
			remote_url: remoteUrl,
			image,
			executor_key: executorKey,
		},
		CONNECT_TIMEOUT_MS,
		secrets,
		deps,
	);
	if (!result.ok) {
		log("docker_executor_connect_failed", {
			workspace,
			session: sessionId,
			error: result.error,
			message: result.message,
			container_id: result.container_id,
			stopped: result.stopped,
			log_tail: result.log_tail,
			owner_session: result.owner_session,
		});
		throw helperError(result);
	}
	log("docker_executor_ready", {
		workspace,
		session: sessionId,
		environment: environmentId,
		reused: result.reused === true,
		container_id: result.container_id,
	});
}

/**
 * Stops and removes the session's container after verifying its ownership
 * labels and that it has exited. Never touches unlabeled containers.
 */
export async function stopDockerWorkspaceExecutor(
	workspace: string,
	sessionId: string,
	deps: DockerDeps = {},
): Promise<void> {
	validateWorkspace(workspace);
	validateId("session id", sessionId);
	const result = await callHelper(
		workspace,
		{ op: "stop", workspace, session_id: sessionId },
		STOP_TIMEOUT_MS,
		[],
		deps,
	);
	if (!result.ok) throw helperError(result);
}

/** Reports the workspace's Docker executor state, e.g. after a controller restart. */
export async function dockerWorkspaceExecutorStatus(
	workspace: string,
	deps: DockerDeps = {},
): Promise<DockerExecutorStatus> {
	validateWorkspace(workspace);
	const result = await callHelper(
		workspace,
		{ op: "status", workspace },
		STATUS_TIMEOUT_MS,
		[],
		deps,
	);
	if (!result.ok) throw helperError(result);
	return {
		status: result.status as ExecutorStatus["status"],
		alive: result.alive === true,
		sessionId:
			typeof result.session_id === "string" ? result.session_id : undefined,
		environmentId:
			typeof result.environment_id === "string"
				? result.environment_id
				: undefined,
		containerId:
			typeof result.container_id === "string" ? result.container_id : undefined,
	};
}

// Coder adapter for the OpenAI webhook prototype.
//
// Connects one preconfigured, already running, dedicated Coder workspace to a
// native OpenAI self-hosted session by launching `codex exec-server` inside a
// bubblewrap allowlist boundary meant for credential-rich templates: nothing
// from the workspace root is visible except read-only system directories,
// selected /etc files, the pinned codex binary, the demo directory, and the
// Codex state directory. /home, /run, /tmp, /var, /sys, /opt, /root, /srv and
// /mnt are absent or empty, and /proc is private to the sandbox PID namespace.
// The adapter never creates, starts, or deletes workspaces
// (`--disable-autostart`).
//
// Secret handling: the executor key travels only over `coder ssh` stdin as one
// JSON line. It is never placed in CLI args, remote files, or logs. Inside the
// workspace it exists in the supervisor's memory and the executor's process
// environment (which is how codex reads CODEX_API_KEY).
//
// Supervision: a fixed Python helper double-forks a detached supervisor (new
// session, SIGHUP ignored, stdio on /dev/null) so the executor survives the SSH
// call and a controller restart. State lives in a control directory outside
// the sandbox's view, and records PID plus kernel start time for
// the supervisor and executor so a recycled PID is never mistaken for ours.
import { spawn } from "node:child_process";

export interface ConnectInput {
	workspace: string;
	environmentId: string;
	remoteUrl: string;
	executorKey: string;
	sessionId: string;
}

export type Log = (kind: string, data: Record<string, unknown>) => void;

export interface RunResult {
	code: number | null;
	stdout: string;
	stderr: string;
}

/** Runs the coder CLI with an argument array, writing stdin and closing it. */
export type CoderRunner = (
	args: string[],
	stdin: string,
	timeoutMs: number,
) => Promise<RunResult>;

export interface CoderDeps {
	runner?: CoderRunner;
}

/** Error reported by the remote helper, e.g. `busy`, `not_owner`, `cleanup_incomplete`. */
export class CoderExecutorError extends Error {
	constructor(
		readonly code: string,
		message: string,
		readonly details: Record<string, unknown> = {},
	) {
		super(message);
		this.name = "CoderExecutorError";
	}
}

export interface ExecutorStatus {
	status:
		| "none"
		| "launching"
		| "starting"
		| "ready"
		| "exited"
		| "stopped"
		| "stale";
	alive: boolean;
	sessionId?: string;
	environmentId?: string;
}

const CONNECT_TIMEOUT_MS = 150_000;
const STOP_TIMEOUT_MS = 90_000;
const STATUS_TIMEOUT_MS = 45_000;
const OUTPUT_LIMIT = 1 << 20;

const NAME_SEGMENT = /^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$/;
const ID_PATTERN = /^[A-Za-z0-9_-]{1,200}$/;
const REMOTE_URL_PATTERN =
	/^https:\/\/api\.openai\.com\/v1\/agents\/api\/connect(?:\/[A-Za-z0-9_-]{1,200}){1,8}$/;

/** Validates a Coder `owner/name` workspace reference. */
export function validateWorkspace(workspace: unknown): string {
	if (typeof workspace !== "string")
		throw new Error("workspace must be owner/name");
	const parts = workspace.split("/");
	if (
		parts.length !== 2 ||
		!parts.every((p) => p.length <= 32 && NAME_SEGMENT.test(p))
	) {
		throw new Error("workspace must be owner/name");
	}
	return workspace;
}

/** Accepts only OpenAI executor connect URLs with plain path segments. */
export function validateRemoteUrl(url: unknown): string {
	if (
		typeof url !== "string" ||
		url.length > 2048 ||
		!REMOTE_URL_PATTERN.test(url)
	) {
		throw new Error("untrusted executor remote URL");
	}
	const parsed = new URL(url);
	if (parsed.href !== url || parsed.origin !== "https://api.openai.com")
		throw new Error("untrusted executor remote URL");
	return url;
}

export function validateId(kind: string, value: unknown): string {
	if (typeof value !== "string" || !ID_PATTERN.test(value))
		throw new Error(`invalid ${kind}`);
	return value;
}

export function validateExecutorKey(key: unknown): string {
	// Never include the value in the error.
	if (
		typeof key !== "string" ||
		key.length < 8 ||
		key.length > 4096 ||
		/\s/.test(key) ||
		[...key].some((character) => character.charCodeAt(0) < 32)
	) {
		throw new Error("invalid executor key");
	}
	return key;
}

/** Replaces every occurrence of each secret in text. */
export function redact(text: string, secrets: readonly string[]): string {
	let out = text;
	for (const s of secrets) if (s) out = out.split(s).join("[REDACTED]");
	return out;
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

// Fixed helper. It receives exactly one JSON line on stdin and prints result
// lines prefixed with "STAGE2 ". It must not contain user-controlled data.
const HELPER = String.raw`
import errno, fcntl, json, os, re, signal, subprocess, sys, time
from pathlib import Path

CONTROL = Path('/home/coder/.codex-stage2-control')
STATE = CONTROL / 'executor.json'
LOCK = CONTROL / 'lock'
LOG = CONTROL / 'executor.log'
CODEX_HOME = '/home/coder/.codex-executor'
WORKDIR = '/home/coder/demo'
RESOLV = CONTROL / 'resolv.conf'
HOST_RESOLV = '/etc/resolv.conf'
BWRAP = '/usr/bin/bwrap'
BINARY = '/home/coder/codex-cli/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex'
READY_MARK = b'Noise executor connected to rendezvous'
READY_TIMEOUT = 90
STOP_TIMEOUT = 15
ID_RE = re.compile(r'^[A-Za-z0-9_-]{1,200}$')
URL_RE = re.compile(r'^https://api\.openai\.com/v1/agents/api/connect(?:/[A-Za-z0-9_-]{1,200}){1,8}$')
ANSI_RE = re.compile(r'\x1b\[[0-9;]*[A-Za-z]')
NAMESERVER_RE = re.compile(r'^nameserver\s+([0-9A-Fa-f.:]{2,45})\s*$')
# Top-level system directories exposed read-only. On usr-merged images these
# are symlinks and are recreated as symlinks instead of bind mounts.
SYSTEM_DIRS = ('usr', 'bin', 'sbin', 'lib', 'lib32', 'lib64', 'libx32')
# /etc entries needed for users, name resolution, dynamic linking, TLS roots,
# and Debian alternatives symlinks. Everything else in /etc is hidden,
# including /etc/ssl/private and any template-provided config.
ETC_ALLOW = ('passwd', 'group', 'nsswitch.conf', 'hosts', 'host.conf', 'localtime', 'ld.so.cache',
             'ld.so.conf', 'ld.so.conf.d', 'ssl/certs', 'ssl/openssl.cnf', 'ca-certificates', 'ca-certificates.conf', 'pki/tls/certs', 'pki/ca-trust/extracted',
             'alternatives', 'os-release', 'protocols', 'services', 'mime.types')

def emit(obj):
    sys.stdout.write('STAGE2 ' + json.dumps(obj, sort_keys=True) + '\n')
    sys.stdout.flush()

def fail(code, message, **extra):
    extra.update(ok=False, error=code, message=message)
    emit(extra)
    sys.exit(3)

def stat_fields(pid):
    try:
        data = Path('/proc/%d/stat' % pid).read_bytes()
    except OSError:
        return None
    rest = data[data.rindex(b')') + 2:].split()
    if rest[0] in (b'Z', b'X'):
        return None
    return int(rest[1]), int(rest[19])

def proc_start(pid):
    f = stat_fields(pid)
    return f[1] if f else None

def alive(pid, start):
    return isinstance(pid, int) and pid > 1 and start is not None and proc_start(pid) == start

def pidns(pid):
    try:
        return os.readlink('/proc/%d/ns/pid' % pid)
    except OSError:
        return None

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

def run_alive(st):
    return bool(st) and (alive(st.get('supervisor_pid'), st.get('supervisor_start'))
                         or alive(st.get('executor_pid'), st.get('executor_start')))

def scan():
    procs = {}
    for d in os.listdir('/proc'):
        if d.isdigit():
            f = stat_fields(int(d))
            if f:
                procs[int(d)] = f
    return procs

def leftovers(st):
    """Processes belonging to the recorded run: its supervisor, executor,
    descendants of the executor, and anything in its private PID namespace."""
    own_ns = pidns(os.getpid())
    ns = st.get('pidns')
    if ns == own_ns:
        ns = None
    ex, ex_start = st.get('executor_pid'), st.get('executor_start')
    ex_alive = alive(ex, ex_start)
    procs = scan()
    out = set()
    for pid, (ppid, start) in procs.items():
        if pid == os.getpid():
            continue
        if (pid == st.get('supervisor_pid') and start == st.get('supervisor_start')) or (pid == ex and start == ex_start):
            out.add(pid)
            continue
        if ns and pidns(pid) == ns:
            out.add(pid)
            continue
        if ex_alive:
            p, hops = ppid, 0
            while p > 1 and hops < 128:
                if p == ex:
                    out.add(pid)
                    break
                p = procs.get(p, (0, 0))[0]
                hops += 1
    return sorted(out)

def child_ns(parent, own_ns):
    for pid, (ppid, _) in scan().items():
        if ppid == parent:
            ns = pidns(pid)
            if ns and ns != own_ns:
                return ns
    return None

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
    deadline = time.time() + 60
    while True:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return fd
        except BlockingIOError:
            if time.time() > deadline:
                fail('lock_timeout', 'another executor operation is in progress')
            time.sleep(0.2)

def terminate_run(st):
    for key in ('executor', 'supervisor'):
        pid, start = st.get(key + '_pid'), st.get(key + '_start')
        if alive(pid, start):
            try:
                os.kill(pid, signal.SIGTERM)
            except OSError:
                pass
    deadline = time.time() + STOP_TIMEOUT
    while time.time() < deadline and leftovers(st):
        time.sleep(0.25)
    for pid in leftovers(st):
        try:
            os.kill(pid, signal.SIGKILL)
        except OSError:
            pass
    deadline = time.time() + 5
    while time.time() < deadline and leftovers(st):
        time.sleep(0.25)
    return leftovers(st)

def write_resolv():
    """Derives the sandbox resolver from the host's nameservers only: no search
    domains (they leak cluster names and caused ndots:5 failures) and the
    stage-1 options. Written outside the sandbox so it cannot be tampered with."""
    try:
        lines = Path(HOST_RESOLV).read_text().splitlines()
    except OSError:
        return False
    servers = [m.group(1) for m in (NAMESERVER_RE.match(l.strip()) for l in lines) if m][:3]
    if not servers:
        return False
    body = ''.join('nameserver %s\n' % s for s in servers) + 'options ndots:1 timeout:3 attempts:2\n'
    tmp = CONTROL / 'resolv.conf.tmp'
    fd = os.open(str(tmp), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
    with os.fdopen(fd, 'w') as f:
        f.write(body)
    os.replace(str(tmp), str(RESOLV))
    return True

def sandbox_args(remote_url, environment_id):
    args = [BWRAP, '--unshare-user', '--unshare-pid', '--unshare-ipc', '--unshare-uts', '--unshare-cgroup-try',
            '--die-with-parent', '--new-session', '--cap-drop', 'ALL', '--hostname', 'codex-executor']
    for d in SYSTEM_DIRS:
        p = '/' + d
        if os.path.islink(p):
            args += ['--symlink', os.readlink(p), p]
        elif os.path.isdir(p):
            args += ['--ro-bind', p, p]
    for e in ETC_ALLOW:
        args += ['--ro-bind-try', '/etc/' + e, '/etc/' + e]
    args += ['--ro-bind', str(RESOLV), '/etc/resolv.conf',
             '--proc', '/proc', '--dev', '/dev',
             '--tmpfs', '/run', '--tmpfs', '/tmp', '--tmpfs', '/var', '--dir', '/var/tmp',
             '--tmpfs', '/home', '--dir', '/home/coder',
             '--ro-bind', BINARY, BINARY,
             '--bind', WORKDIR, WORKDIR, '--bind', CODEX_HOME, CODEX_HOME,
             '--chdir', WORKDIR, '--', BINARY, 'exec-server',
             '--remote', remote_url, '--environment-id', environment_id]
    return args

def supervise(cfg, nonce):
    signal.signal(signal.SIGHUP, signal.SIG_IGN)
    devnull = os.open('/dev/null', os.O_RDWR)
    for fd in (0, 1, 2):
        os.dup2(devnull, fd)
    os.closerange(3, 65536)
    os.umask(0o077)
    me = os.getpid()
    own_ns = pidns(me)
    # A newer run replaced this one (e.g. the launching helper died): never start an untracked executor.
    if not update(nonce, supervisor_pid=me, supervisor_start=proc_start(me), status='starting'):
        return
    key = cfg['executor_key']
    env = {'HOME': '/home/coder', 'PATH': '/usr/local/bin:/usr/bin:/bin',
           'CODEX_HOME': CODEX_HOME, 'CODEX_API_KEY': key, 'RUST_LOG': 'info'}
    args = sandbox_args(cfg['remote_url'], cfg['environment_id'])
    keyb = key.encode()
    logf = open(str(LOG), 'wb', buffering=0)
    try:
        proc = subprocess.Popen(args, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=subprocess.STDOUT, env=env, close_fds=True, cwd=WORKDIR)
    except OSError as e:
        update(nonce, status='exited', exit_code=None, launch_error=e.__class__.__name__)
        return
    env = cfg = key = None
    signal.signal(signal.SIGTERM, lambda *_: proc.terminate())
    if not update(nonce, executor_pid=proc.pid, executor_start=proc_start(proc.pid)):
        proc.kill()
    ready = False
    ns = None
    for line in proc.stdout:
        if keyb in line:
            line = line.replace(keyb, b'[REDACTED]')
        logf.write(line)
        if ns is None:
            ns = child_ns(proc.pid, own_ns)
            if ns:
                update(nonce, pidns=ns)
        if not ready and READY_MARK in line:
            ready = True
            update(nonce, status='ready', ready_at=time.time())
    rc = proc.wait()
    update(nonce, status='exited', exit_code=rc, exited_at=time.time())

def spawn(cfg, nonce):
    pid = os.fork()
    if pid == 0:
        try:
            os.setsid()
            if os.fork() == 0:
                supervise(cfg, nonce)
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
        if st.get('status') == 'ready' and run_alive(st):
            emit({'ok': True, 'status': 'ready', 'reused': reused, 'executor_pid': st.get('executor_pid')})
            return
        if st.get('status') == 'exited' or (st.get('supervisor_pid') and not run_alive(st)):
            terminate_run(st)
            fail('executor_exited', 'executor exited before rendezvous', exit_code=st.get('exit_code'), log_tail=log_tail())
        if not st.get('supervisor_pid') and time.time() - t0 > 15:
            st['nonce'] = 'abandoned-' + nonce
            st['status'] = 'exited'
            write_state(st)
            fail('supervisor_missing', 'detached supervisor did not start')
        time.sleep(0.25)
    st = read_state() or {}
    left = terminate_run(st)
    fail('ready_timeout', 'executor did not reach rendezvous in time', log_tail=log_tail(), leftovers=left)

def check_ids(cfg, names):
    for n in names:
        if not isinstance(cfg.get(n), str) or not ID_RE.match(cfg[n]):
            fail('invalid_input', 'invalid ' + n)

def connect(cfg):
    check_ids(cfg, ('session_id', 'environment_id'))
    if not isinstance(cfg.get('remote_url'), str) or not URL_RE.match(cfg['remote_url']):
        fail('invalid_input', 'untrusted remote URL')
    key = cfg.get('executor_key')
    if not isinstance(key, str) or len(key) < 8 or re.search(r'[\s\x00-\x1f]', key):
        fail('invalid_input', 'invalid executor key')
    lock()
    st = read_state()
    if run_alive(st):
        if st.get('session_id') != cfg['session_id']:
            fail('busy', 'executor is owned by another session', owner_session=st.get('session_id'))
        if st.get('environment_id') != cfg['environment_id'] or st.get('remote_url') != cfg['remote_url']:
            fail('conflict', 'running executor has a different environment for this session')
        if st.get('status') == 'ready':
            emit({'ok': True, 'status': 'ready', 'reused': True, 'executor_pid': st.get('executor_pid')})
            return
        wait_ready(st['nonce'], True)
        return
    if st:
        left = leftovers(st)
        if left:
            fail('stale_processes', 'previous executor run still has processes', leftovers=left)
    if not os.path.isdir(CODEX_HOME):
        os.mkdir(CODEX_HOME, 0o700)
    if not write_resolv():
        fail('not_prepared', 'no usable nameserver in ' + HOST_RESOLV)
    for path, kind in ((BWRAP, 'x'), (BINARY, 'x'), (WORKDIR, 'd'), (CODEX_HOME, 'd')):
        ok = os.path.isdir(path) if kind == 'd' else (os.access(path, os.X_OK) if kind == 'x' else os.path.isfile(path))
        if not ok:
            fail('not_prepared', 'workspace is missing prepared executor path ' + path)
    nonce = os.urandom(16).hex()
    write_state({'nonce': nonce, 'session_id': cfg['session_id'], 'environment_id': cfg['environment_id'],
                 'remote_url': cfg['remote_url'], 'status': 'launching', 'created_at': time.time()})
    spawn(cfg, nonce)
    cfg['executor_key'] = None
    wait_ready(nonce, False)

def stop(cfg):
    check_ids(cfg, ('session_id',))
    lock()
    st = read_state()
    if not st:
        emit({'ok': True, 'status': 'not_running', 'leftovers': []})
        return
    owner = st.get('session_id') == cfg['session_id']
    if run_alive(st) and not owner:
        fail('not_owner', 'executor is owned by another session', owner_session=st.get('session_id'))
    if not run_alive(st):
        left = leftovers(st)
        if left and not owner:
            fail('not_owner', 'leftover processes belong to another session', owner_session=st.get('session_id'))
        if not left:
            if owner and st.get('status') != 'stopped':
                st['status'] = 'stopped'
                write_state(st)
            emit({'ok': True, 'status': 'not_running', 'leftovers': []})
            return
    left = terminate_run(st)
    if left:
        fail('cleanup_incomplete', 'executor processes remain after stop', leftovers=left)
    cur = read_state()
    if cur and cur.get('nonce') == st.get('nonce'):
        cur['status'] = 'stopped'
        cur['stopped_at'] = time.time()
        write_state(cur)
    emit({'ok': True, 'status': 'stopped', 'leftovers': []})

def status(cfg):
    st = read_state()
    if not st:
        emit({'ok': True, 'status': 'none', 'alive': False})
        return
    live = run_alive(st)
    state = st.get('status')
    if live and state not in ('launching', 'starting', 'ready'):
        state = 'stale'
    if not live and state in ('launching', 'starting', 'ready'):
        state = 'stale'
    emit({'ok': True, 'status': state, 'alive': live, 'session_id': st.get('session_id'),
          'environment_id': st.get('environment_id')})

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
export const REMOTE_HELPER_SOURCE = HELPER;

/** The exact, constant remote command passed after `--` to `coder ssh`. */
export const REMOTE_COMMAND = `python3 -c "import base64;exec(base64.b64decode('${Buffer.from(HELPER).toString("base64")}'))"`;

/** Builds the `coder ssh` argument array for a validated workspace. */
export function buildSshArgs(workspace: string): string[] {
	return [
		"ssh",
		"--disable-autostart",
		validateWorkspace(workspace),
		"--",
		REMOTE_COMMAND,
	];
}

export interface HelperResult {
	ok: boolean;
	error?: string;
	message?: string;
	[key: string]: unknown;
}

/** Returns the last well-formed `STAGE2 {...}` line from helper stdout. */
export function parseHelperOutput(stdout: string): HelperResult | undefined {
	let result: HelperResult | undefined;
	for (const line of stdout.split("\n")) {
		if (!line.startsWith("STAGE2 ")) continue;
		try {
			const parsed = JSON.parse(line.slice(7));
			if (
				parsed &&
				typeof parsed === "object" &&
				typeof parsed.ok === "boolean"
			)
				result = parsed;
		} catch {
			// Ignore malformed lines; absence of a result is handled by the caller.
		}
	}
	return result;
}

// Environment passed to the coder CLI. The controller's OpenAI credentials are
// deliberately excluded.
const ENV_ALLOW =
	/^(?:CODER_[A-Z0-9_]+|HOME|PATH|USER|LANG|LC_[A-Z]+|TMPDIR|XDG_[A-Z_]+|SSL_CERT_FILE|SSL_CERT_DIR|HTTPS?_PROXY|NO_PROXY)$/;

export function coderEnv(
	source: NodeJS.ProcessEnv = process.env,
): NodeJS.ProcessEnv {
	return Object.fromEntries(
		Object.entries(source).filter(
			([k, v]) => ENV_ALLOW.test(k) && v !== undefined,
		),
	);
}

export const defaultRunner: CoderRunner = (args, stdin, timeoutMs) =>
	new Promise((resolve, reject) => {
		const child = spawn(process.env.CODER_BIN ?? "coder", args, {
			stdio: ["pipe", "pipe", "pipe"],
			env: coderEnv(),
			shell: false,
		});
		let stdout = "";
		let stderr = "";
		child.stdout.setEncoding("utf8").on("data", (d: string) => {
			if (stdout.length < OUTPUT_LIMIT) stdout += d;
		});
		child.stderr.setEncoding("utf8").on("data", (d: string) => {
			if (stderr.length < OUTPUT_LIMIT) stderr += d;
		});
		const timer = setTimeout(() => child.kill("SIGTERM"), timeoutMs);
		child.on("error", (err) => {
			clearTimeout(timer);
			reject(err);
		});
		child.on("close", (code) => {
			clearTimeout(timer);
			resolve({ code, stdout, stderr });
		});
		child.stdin.on("error", () => {});
		child.stdin.end(stdin);
	});

async function callHelper(
	workspace: string,
	payload: Record<string, unknown>,
	timeoutMs: number,
	secrets: string[],
	deps: CoderDeps,
): Promise<HelperResult> {
	const runner = deps.runner ?? defaultRunner;
	const res = await runner(
		buildSshArgs(workspace),
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
		String(message ?? "executor helper failed"),
		details,
	);
}

/**
 * Starts (or reattaches to) the dedicated workspace's executor for a session.
 * Resolves only after the executor reports rendezvous. Idempotent for the same
 * session; rejects with code `busy` if another session owns the executor.
 */
export async function connectWorkspace(
	input: ConnectInput,
	log: Log,
	deps: CoderDeps = {},
): Promise<void> {
	const workspace = validateWorkspace(input.workspace);
	const sessionId = validateId("session id", input.sessionId);
	const environmentId = validateId("environment id", input.environmentId);
	const remoteUrl = validateRemoteUrl(input.remoteUrl);
	const executorKey = validateExecutorKey(input.executorKey);
	const secrets = [executorKey];
	log("coder_executor_connect", {
		workspace,
		session: sessionId,
		environment: environmentId,
	});
	const result = await callHelper(
		workspace,
		{
			op: "connect",
			session_id: sessionId,
			environment_id: environmentId,
			remote_url: remoteUrl,
			executor_key: executorKey,
		},
		CONNECT_TIMEOUT_MS,
		secrets,
		deps,
	);
	if (!result.ok) {
		log("coder_executor_connect_failed", {
			workspace,
			session: sessionId,
			error: result.error,
			message: result.message,
			log_tail: result.log_tail,
			owner_session: result.owner_session,
		});
		throw helperError(result);
	}
	log("coder_executor_ready", {
		workspace,
		session: sessionId,
		environment: environmentId,
		reused: result.reused === true,
		executor_pid: result.executor_pid,
	});
}

/**
 * Stops the executor if this session owns it and verifies no process from its
 * boundary remains. Never stops or deletes the workspace.
 */
export async function stopWorkspaceExecutor(
	workspace: string,
	sessionId: string,
	deps: CoderDeps = {},
): Promise<void> {
	validateWorkspace(workspace);
	validateId("session id", sessionId);
	const result = await callHelper(
		workspace,
		{ op: "stop", session_id: sessionId },
		STOP_TIMEOUT_MS,
		[],
		deps,
	);
	if (!result.ok) throw helperError(result);
}

/** Reports the dedicated workspace's executor state, e.g. after a controller restart. */
export async function workspaceExecutorStatus(
	workspace: string,
	deps: CoderDeps = {},
): Promise<ExecutorStatus> {
	validateWorkspace(workspace);
	const result = await callHelper(
		workspace,
		{ op: "status" },
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
	};
}

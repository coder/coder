import { execFile, spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import {
	linkSync,
	mkdirSync,
	readFileSync,
	renameSync,
	unlinkSync,
	writeFileSync,
} from "node:fs";
import { join } from "node:path";
import { promisify } from "node:util";
import {
	type CoderRunner,
	coderEnv,
	type HelperResult,
	type Log,
	parseHelperOutput,
	redact,
	validateId,
	validateWorkspace,
} from "./coder.ts";

const run = promisify(execFile);
/** Start only the preconfigured, prepared worker. Never create arbitrary templates here. */
export async function ensureWorkspaceRunning(workspace: string): Promise<void> {
	validateWorkspace(workspace);
	await run("coder", ["start", workspace, "--yes"], {
		env: coderEnv(),
		timeout: 180000,
		maxBuffer: 1024 * 1024,
	});
}

// On-demand provisioning of exactly one worker workspace from the fixed `coder`
// template. Ownership is proven by a local claim written before the create
// request (only after the name was verified absent) plus the workspace ID
// recorded once the workspace is seen, and inside the workspace by a receipt
// bound to the claim. An existing workspace without our claim is never adopted.
//
// Creation uses the coderd REST API rather than `coder create`, which queues a
// template dry-run job before creating anything and leaves that job behind
// when killed.

export const PROVISION_TEMPLATE = "coder";
export const PROVISION_ORGANIZATION = "coder";
/** Allowlisted ID of the `coder` template; discovery must resolve to exactly this template. */
export const CODER_TEMPLATE_ID = "0d286645-29aa-4eaf-9b52-cc5d2740c90b";
const WORKSPACE_TTL_MS = 2 * 60 * 60 * 1000;
export const CODEX_VERSION = "0.156.0-alpha.2";
export const EXECUTOR_IMAGE_TAG = "codex-webhook-executor:stage2";

const IMAGE_ID = /^sha256:[0-9a-f]{64}$/;
const CLAIM = /^[0-9a-f]{32}$/;
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const OUTPUT_LIMIT = 1 << 20;
const API_TIMEOUT_MS = 60_000;
const CHECK_TIMEOUT_MS = 90_000;
const PREPARE_TIMEOUT_MS = 1_200_000;
const DEFAULT_TIMEOUT_MS = 1_500_000;
const DEFAULT_POLL_MS = 5_000;
// The pre-create absence check proves the name was free at absentCheckedAt;
// this allows for clock skew between the controller and coderd.
const CLOCK_SKEW_MS = 60_000;

export class ProvisionError extends Error {
	constructor(
		readonly code: string,
		message: string,
	) {
		super(message);
		this.name = "ProvisionError";
	}
}

/** The subset of a Coder workspace used to prove ownership and readiness. */
export interface CoderWorkspace {
	id: string;
	ownerName: string;
	name: string;
	templateId: string;
	templateName: string;
	createdAt: string;
	status: string;
}

export interface ProvisionState {
	version: 1;
	workspace: string;
	sessionId: string;
	claim: string;
	template: string;
	absentCheckedAt: string;
	phase: "intent" | "created" | "prepared";
	workspaceId?: string;
	image?: string;
}

export interface ProvisionDeps {
	/** Runs the coder CLI. Default: `coder` with a provisioning-only environment, shell disabled. */
	runner?: CoderRunner;
	/** Workspace lookup by owner/name; null when absent. Default: coderd REST API. */
	getWorkspace?: (
		owner: string,
		name: string,
	) => Promise<CoderWorkspace | null>;
	/** Creates the workspace from the allowlisted template. Default: createWorkspaceViaApi. */
	createWorkspace?: (owner: string, name: string) => Promise<CoderWorkspace>;
	/** Directory holding provision.json. Default: $STATE_DIR or ../state. */
	stateDir?: string;
	/** Executor build inputs. Default: Dockerfile.executor and executor-entrypoint.py at the project root. */
	assets?: { dockerfile: string; entrypoint: string };
	now?: () => number;
	sleep?: (ms: number) => Promise<void>;
	randomClaim?: () => string;
	/** Overall deadline for one call. */
	timeoutMs?: number;
	pollMs?: number;
}

export interface CreateWorkspaceBody {
	template_id: string;
	name: string;
	ttl_ms: number;
	rich_parameter_values: { name: string; value: string }[];
	template_version_preset_id?: string;
}

/**
 * Fixed create request: allowlisted template, `Select IDEs=[]`, 2h stop, and
 * the template's default preset (as `coder create` would pick). Other
 * parameters take template defaults server-side.
 */
export function buildCreateBody(
	name: string,
	presetId?: string,
): CreateWorkspaceBody {
	validateWorkspace(`x/${name}`);
	if (presetId !== undefined && !UUID.test(presetId))
		throw new ProvisionError("api_invalid", "invalid preset ID");
	return {
		template_id: CODER_TEMPLATE_ID,
		name,
		ttl_ms: WORKSPACE_TTL_MS,
		rich_parameter_values: [{ name: "Select IDEs", value: "[]" }],
		...(presetId ? { template_version_preset_id: presetId } : {}),
	};
}

// Only connection and auth variables reach the CLI, so CODER_* variables that
// map to create flags (template version, parameters, presets, org) cannot alter the build.
const PROVISION_ENV = new Set([
	"CODER_URL",
	"CODER_SESSION_TOKEN",
	"CODER_CONFIG_DIR",
]);

export function provisionEnv(
	source: NodeJS.ProcessEnv = process.env,
): NodeJS.ProcessEnv {
	return Object.fromEntries(
		Object.entries(coderEnv(source)).filter(
			([k]) => !k.startsWith("CODER_") || PROVISION_ENV.has(k),
		),
	);
}

const provisionRunner: CoderRunner = (args, stdin, timeoutMs) =>
	new Promise((resolve, reject) => {
		const child = spawn(process.env.CODER_BIN ?? "coder", args, {
			stdio: ["pipe", "pipe", "pipe"],
			env: provisionEnv(),
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

/** Validates a coderd workspace response and keeps only the fields we rely on. */
export function parseWorkspace(body: unknown): CoderWorkspace {
	const w = body as Record<string, unknown> | null;
	const build = w?.latest_build as Record<string, unknown> | undefined;
	const fields = {
		id: w?.id,
		ownerName: w?.owner_name,
		name: w?.name,
		templateId: w?.template_id,
		templateName: w?.template_name,
		createdAt: w?.created_at,
		status: build?.status,
	};
	if (
		!Object.values(fields).every((v) => typeof v === "string") ||
		!UUID.test(fields.id as string) ||
		!UUID.test(fields.templateId as string) ||
		Number.isNaN(Date.parse(fields.createdAt as string))
	) {
		throw new ProvisionError(
			"api_invalid",
			"coderd returned an unexpected workspace shape",
		);
	}
	return fields as CoderWorkspace;
}

// Calls coderd with the session token as a header only. Paths are built from
// validated names and UUIDs.
async function coderApi(
	env: NodeJS.ProcessEnv,
	fetchImpl: typeof fetch,
	method: "GET" | "POST",
	path: string,
	body?: unknown,
): Promise<{ status: number; json: unknown }> {
	const base = env.CODER_URL;
	const token = env.CODER_SESSION_TOKEN;
	if (!base || !token || !/^https?:$/.test(new URL(base).protocol))
		throw new ProvisionError(
			"api_unconfigured",
			"CODER_URL and CODER_SESSION_TOKEN are required",
		);
	const headers: Record<string, string> = {
		"Coder-Session-Token": token,
		Accept: "application/json",
	};
	if (body !== undefined) headers["Content-Type"] = "application/json";
	const res = await fetchImpl(`${base.replace(/\/+$/, "")}${path}`, {
		method,
		headers,
		body: body === undefined ? undefined : JSON.stringify(body),
		redirect: "error",
		signal: AbortSignal.timeout(API_TIMEOUT_MS),
	});
	const text = await res.text();
	let json: unknown;
	try {
		json = text ? JSON.parse(text) : undefined;
	} catch {
		json = undefined;
	}
	return { status: res.status, json };
}

function apiMessage(json: unknown, token: string | undefined): string {
	const m = (json as { message?: unknown } | undefined)?.message;
	return typeof m === "string" ? redact(m, [token ?? ""]).slice(0, 300) : "";
}

/** GET /api/v2/users/{owner}/workspace/{name}. The token is sent only as a header and never logged. */
export async function fetchWorkspace(
	owner: string,
	name: string,
	env: NodeJS.ProcessEnv = process.env,
	fetchImpl: typeof fetch = fetch,
): Promise<CoderWorkspace | null> {
	validateWorkspace(`${owner}/${name}`);
	const res = await coderApi(
		env,
		fetchImpl,
		"GET",
		`/api/v2/users/${owner}/workspace/${name}`,
	);
	if (res.status === 404) return null;
	if (res.status !== 200)
		throw new ProvisionError(
			"api_failed",
			`workspace lookup failed: HTTP ${res.status}`,
		);
	return parseWorkspace(res.json);
}

/**
 * Verifies the caller is `owner` and a member of the `coder` organization, that
 * the org's `coder` template is the allowlisted, non-deprecated template, picks
 * its active version's default preset, then
 * POST /api/v2/organizations/{org}/members/{owner}/workspaces.
 * Throws `create_conflict` on 409 and `create_rejected` on other 4xx.
 */
export async function createWorkspaceViaApi(
	owner: string,
	name: string,
	env: NodeJS.ProcessEnv = process.env,
	fetchImpl: typeof fetch = fetch,
): Promise<CoderWorkspace> {
	validateWorkspace(`${owner}/${name}`);
	const get = async (path: string, what: string) => {
		const res = await coderApi(env, fetchImpl, "GET", path);
		if (res.status !== 200 || !res.json || typeof res.json !== "object")
			throw new ProvisionError(
				"api_failed",
				`${what} lookup failed: HTTP ${res.status}`,
			);
		return res.json as Record<string, unknown>;
	};
	const me = await get("/api/v2/users/me", "user");
	const org = await get(
		`/api/v2/organizations/${PROVISION_ORGANIZATION}`,
		"organization",
	);
	if (
		org.name !== PROVISION_ORGANIZATION ||
		typeof org.id !== "string" ||
		!UUID.test(org.id)
	)
		throw new ProvisionError("api_invalid", "unexpected organization");
	if (
		me.username !== owner ||
		!Array.isArray(me.organization_ids) ||
		!me.organization_ids.includes(org.id)
	) {
		throw new ProvisionError(
			"not_owner",
			"authenticated user is not the workspace owner in the coder organization",
		);
	}
	const tpl = await get(
		`/api/v2/organizations/${org.id}/templates/${PROVISION_TEMPLATE}`,
		"template",
	);
	if (
		tpl.id !== CODER_TEMPLATE_ID ||
		tpl.organization_id !== org.id ||
		tpl.deprecated === true ||
		typeof tpl.active_version_id !== "string" ||
		!UUID.test(tpl.active_version_id)
	) {
		throw new ProvisionError(
			"template_mismatch",
			"coder template is not the allowlisted template",
		);
	}
	const presetsRes = await coderApi(
		env,
		fetchImpl,
		"GET",
		`/api/v2/templateversions/${tpl.active_version_id}/presets`,
	);
	if (presetsRes.status !== 200)
		throw new ProvisionError(
			"api_failed",
			`preset lookup failed: HTTP ${presetsRes.status}`,
		);
	const defaults = (
		Array.isArray(presetsRes.json) ? presetsRes.json : []
	).filter((p: { Default?: unknown }) => p?.Default === true);
	if (defaults.length > 1)
		throw new ProvisionError(
			"api_invalid",
			"template has more than one default preset",
		);
	const body = buildCreateBody(name, defaults[0]?.ID);
	const res = await coderApi(
		env,
		fetchImpl,
		"POST",
		`/api/v2/organizations/${org.id}/members/${owner}/workspaces`,
		body,
	);
	if (res.status === 201) return parseWorkspace(res.json);
	const detail = `HTTP ${res.status}${apiMessage(res.json, env.CODER_SESSION_TOKEN) ? `: ${apiMessage(res.json, env.CODER_SESSION_TOKEN)}` : ""}`;
	if (res.status === 409)
		throw new ProvisionError(
			"create_conflict",
			`workspace create conflict: ${detail}`,
		);
	if (res.status >= 400 && res.status < 500)
		throw new ProvisionError(
			"create_rejected",
			`workspace create rejected: ${detail}`,
		);
	throw new ProvisionError(
		"create_failed",
		`workspace create failed: ${detail}`,
	);
}

function statePath(dir: string): string {
	return join(dir, "provision.json");
}

export function readProvisionState(dir: string): ProvisionState | undefined {
	let text: string;
	try {
		text = readFileSync(statePath(dir), "utf8");
	} catch (err) {
		if ((err as NodeJS.ErrnoException).code === "ENOENT") return undefined;
		throw err;
	}
	const s = JSON.parse(text) as ProvisionState;
	if (
		s?.version !== 1 ||
		typeof s.workspace !== "string" ||
		typeof s.sessionId !== "string" ||
		!CLAIM.test(String(s.claim)) ||
		s.template !== PROVISION_TEMPLATE
	) {
		throw new ProvisionError("state_corrupt", "provision.json is invalid");
	}
	return s;
}

// Atomic write. `exclusive` creates the file only if absent (link fails on EEXIST).
function writeState(
	dir: string,
	state: ProvisionState,
	exclusive: boolean,
): void {
	mkdirSync(dir, { recursive: true, mode: 0o700 });
	const tmp = `${statePath(dir)}.${randomBytes(6).toString("hex")}.tmp`;
	writeFileSync(tmp, `${JSON.stringify(state, null, 2)}\n`, {
		mode: 0o600,
		flag: "wx",
	});
	try {
		if (exclusive) linkSync(tmp, statePath(dir));
		else renameSync(tmp, statePath(dir));
	} finally {
		try {
			unlinkSync(tmp);
		} catch {
			/* renamed */
		}
	}
}

function owns(ws: CoderWorkspace, state: ProvisionState): boolean {
	const [owner, name] = state.workspace.split("/");
	if (
		ws.ownerName !== owner ||
		ws.name !== name ||
		ws.templateId !== CODER_TEMPLATE_ID
	)
		return false;
	if (state.workspaceId) return ws.id === state.workspaceId;
	return (
		Date.parse(ws.createdAt) >=
		Date.parse(state.absentCheckedAt) - CLOCK_SKEW_MS
	);
}

function tail(text: string): string {
	const secrets = [
		process.env.CODER_SESSION_TOKEN ?? "",
		process.env.CODER_AGENT_TOKEN ?? "",
	];
	return redact(text, secrets)
		.split("\n")
		.filter(Boolean)
		.slice(-5)
		.join(" | ")
		.slice(-800);
}

// Fixed helper run over `coder ssh`. It receives one JSON line on stdin and
// prints result lines prefixed with "STAGE2 ". Subprocesses get an allowlisted
// environment, so workspace tokens never reach npm or docker build.
const HELPER = String.raw`
import fcntl, hashlib, json, os, re, shutil, subprocess, sys, tempfile, time

HOME = '/home/coder'
RECEIPT = HOME + '/.codex-stage2-prepared.json'
LOCK = HOME + '/.codex-stage2-prepare.lock'
DEMO = HOME + '/demo'
MARKER = DEMO + '/worker-marker.txt'
CLI = HOME + '/codex-cli'
BUILD = HOME + '/.codex-stage2-build'
BINARY = CLI + '/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex'
PKG = CLI + '/node_modules/@openai/codex/package.json'
CODEX_VERSION = '0.156.0-alpha.2'
DOCKER = '/usr/bin/docker'
TAG = 'codex-webhook-executor:stage2'
ENTRYPOINT = ['python3', '/usr/local/bin/executor-entrypoint.py']
ID_RE = re.compile(r'^[A-Za-z0-9_-]{1,200}$')
WS_RE = re.compile(r'^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*/[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$')
HEX_RE = re.compile(r'^[0-9a-f]{32}$')
IMAGE_RE = re.compile(r'^sha256:[0-9a-f]{64}$')
ANSI_RE = re.compile(r'\x1b\[[0-9;]*[A-Za-z]')
ENV_ALLOW = ('HOME', 'PATH', 'USER', 'LANG', 'SSL_CERT_FILE', 'SSL_CERT_DIR', 'HTTP_PROXY', 'HTTPS_PROXY', 'NO_PROXY',
             'DOCKER_HOST', 'DOCKER_CONFIG', 'DOCKER_CONTEXT', 'DOCKER_CERT_PATH', 'DOCKER_TLS_VERIFY')
OUT_LIMIT = 4000

def emit(obj):
    sys.stdout.write('STAGE2 ' + json.dumps(obj, sort_keys=True) + '\n')
    sys.stdout.flush()

def fail(code, message, **extra):
    extra.update(ok=False, error=code, message=message)
    emit(extra)
    sys.exit(3)

def run(args, timeout):
    """Returns (exit code or 'timeout'/'oserror', bounded output tail)."""
    env = {k: os.environ[k] for k in ENV_ALLOW if k in os.environ}
    with tempfile.TemporaryFile() as out:
        try:
            rc = subprocess.run(args, stdin=subprocess.DEVNULL, stdout=out, stderr=subprocess.STDOUT,
                                env=env, cwd=HOME, timeout=timeout).returncode
        except subprocess.TimeoutExpired:
            rc = 'timeout'
        except OSError as e:
            return 'oserror', e.__class__.__name__
        out.seek(0, 2)
        out.seek(max(0, out.tell() - OUT_LIMIT))
        return rc, ANSI_RE.sub('', out.read().decode('utf-8', 'replace'))

def atomic_write(path, body, mode):
    tmp = '%s.%d.tmp' % (path, os.getpid())
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
    with os.fdopen(fd, 'w') as f:
        f.write(body)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)

def check(cfg):
    missing = []
    npm = shutil.which('npm')
    if not npm or run([npm, '--version'], 60)[0] != 0:
        missing.append('npm')
    if not os.access(DOCKER, os.X_OK) or run([DOCKER, 'info', '--format', '{{.ServerVersion}}'], 30)[0] != 0:
        missing.append('docker')
    if missing:
        fail('not_ready', 'workspace is missing ' + ', '.join(missing), missing=missing)
    emit({'ok': True, 'missing': []})

def claim_receipt(cfg):
    try:
        r = json.loads(open(RECEIPT).read())
    except FileNotFoundError:
        r = None
    except (OSError, ValueError):
        fail('receipt_corrupt', 'prepare receipt is unreadable')
    if r is not None:
        if not isinstance(r, dict) or any(r.get(k) != cfg[k] for k in ('workspace', 'session_id', 'claim')) \
                or not HEX_RE.match(str(r.get('marker'))):
            fail('not_owner', 'workspace was prepared for a different claim')
        return r, True
    if os.path.lexists(DEMO):
        fail('not_owner', DEMO + ' exists without a prepare receipt')
    r = {'workspace': cfg['workspace'], 'session_id': cfg['session_id'], 'claim': cfg['claim'],
         'marker': os.urandom(16).hex(), 'created_at': time.time()}
    atomic_write(RECEIPT, json.dumps(r) + '\n', 0o600)
    return r, False

def ensure_demo(marker):
    if os.path.islink(DEMO):
        fail('not_owner', DEMO + ' is a symlink')
    os.makedirs(DEMO, 0o700, exist_ok=True)
    body = 'codex-stage2 worker marker %s\n' % marker
    try:
        current = open(MARKER).read() if not os.path.islink(MARKER) else None
    except FileNotFoundError:
        current = None
    if current != body:
        if os.path.islink(MARKER):
            os.unlink(MARKER)
        atomic_write(MARKER, body, 0o600)

def installed():
    try:
        version = json.loads(open(PKG).read()).get('version')
    except (OSError, ValueError, AttributeError):
        return False
    return version == CODEX_VERSION and os.path.isfile(BINARY) and os.access(BINARY, os.X_OK)

def install_codex():
    if installed():
        return True
    npm = shutil.which('npm')
    if not npm:
        fail('install_failed', 'npm is not available')
    os.makedirs(CLI, 0o755, exist_ok=True)
    rc, out = run([npm, 'install', '--prefix', CLI, '--no-audit', '--no-fund', '--ignore-scripts', '--save-exact',
                   '@openai/codex@' + CODEX_VERSION], 600)
    if rc != 0 or not installed():
        fail('install_failed', 'npm install of pinned codex failed', exit_code=rc, tail=out[-1500:])
    rc, out = run([BINARY, '--version'], 30)
    if rc != 0:
        fail('install_failed', 'installed codex binary does not run', exit_code=rc, tail=out[-500:])
    return False

def build_image(files):
    if os.path.islink(BUILD):
        fail('build_failed', BUILD + ' is a symlink')
    os.makedirs(BUILD, 0o700, exist_ok=True)
    for name, body in files.items():
        atomic_write(BUILD + '/' + name, body, 0o644)
    iid = BUILD + '/image-id'
    if os.path.lexists(iid):
        os.unlink(iid)
    rc, out = run([DOCKER, 'build', '-f', BUILD + '/Dockerfile.executor', '-t', TAG, '--iidfile', iid, BUILD], 900)
    if rc != 0:
        fail('build_failed', 'docker build failed', exit_code=rc, tail=out[-1500:])
    try:
        image = open(iid).read().strip()
    except OSError:
        image = ''
    if not IMAGE_RE.match(image):
        fail('build_failed', 'docker build produced an invalid image ID')
    rc, out = run([DOCKER, 'image', 'inspect', '--format', '{{.Id}}|{{json .Config.Entrypoint}}', image], 30)
    got_id, _, entry = out.strip().partition('|')
    try:
        entry = json.loads(entry)
    except ValueError:
        entry = None
    if rc != 0 or got_id != image or entry != ENTRYPOINT:
        fail('build_failed', 'built image does not match the executor image')
    return image

def prepare(cfg):
    for key, rx in (('workspace', WS_RE), ('session_id', ID_RE), ('claim', HEX_RE)):
        if not isinstance(cfg.get(key), str) or not rx.match(cfg[key]):
            fail('invalid_input', 'invalid ' + key)
    files = {}
    for key, name in (('dockerfile', 'Dockerfile.executor'), ('entrypoint', 'executor-entrypoint.py')):
        body = cfg.get(key)
        if not isinstance(body, str) or not body or len(body) > 65536:
            fail('invalid_input', 'invalid ' + key)
        files[name] = body
    fd = os.open(LOCK, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        fail('busy', 'another prepare is in progress')
    receipt, reused = claim_receipt(cfg)
    ensure_demo(receipt['marker'])
    codex_reused = install_codex()
    image = build_image(files)
    emit({'ok': True, 'image': image, 'receipt_reused': reused, 'codex_reused': codex_reused})

def main():
    try:
        cfg = json.loads(sys.stdin.readline() or '{}')
    except ValueError:
        fail('invalid_input', 'stdin must be one JSON line')
    ops = {'check': check, 'prepare': prepare}
    op = cfg.get('op') if isinstance(cfg, dict) else None
    if op not in ops:
        fail('invalid_input', 'unknown op')
    ops[op](cfg)

main()
`;

/** Source of the fixed prepare helper, exported for tests. */
export const PREPARE_HELPER_SOURCE = HELPER;

/** The exact, constant remote command passed after `--` to `coder ssh`. */
export const PREPARE_REMOTE_COMMAND = `python3 -c "import base64;exec(base64.b64decode('${Buffer.from(HELPER).toString("base64")}'))"`;

/** `coder ssh` arguments: never wait for startup scripts and never autostart. */
export function buildPrepareSshArgs(workspace: string): string[] {
	return [
		"ssh",
		"--wait=no",
		"--disable-autostart",
		validateWorkspace(workspace),
		"--",
		PREPARE_REMOTE_COMMAND,
	];
}

function loadAssets(): { dockerfile: string; entrypoint: string } {
	const root = join(import.meta.dirname, "..");
	return {
		dockerfile: readFileSync(join(root, "Dockerfile.executor"), "utf8"),
		entrypoint: readFileSync(join(root, "executor-entrypoint.py"), "utf8"),
	};
}

/**
 * Ensures the single on-demand worker workspace exists, is ours, is running,
 * and has the pinned codex CLI, demo marker, and executor image. Creates the
 * workspace from the fixed `coder` template only when the name is absent and
 * no claim exists; retries reuse the claimed workspace. Returns the immutable
 * executor image ID. Call only after a verified webhook.
 */
export async function prepareOnDemandWorkspace(
	input: { workspace: string; sessionId: string },
	log: Log,
	deps: ProvisionDeps = {},
): Promise<string> {
	const workspace = validateWorkspace(input.workspace);
	const sessionId = validateId("session id", input.sessionId);
	const [owner, name] = workspace.split("/");
	const runner = deps.runner ?? provisionRunner;
	const lookup =
		deps.getWorkspace ?? ((o: string, n: string) => fetchWorkspace(o, n));
	const create =
		deps.createWorkspace ??
		((o: string, n: string) => createWorkspaceViaApi(o, n));
	const dir =
		deps.stateDir ??
		process.env.STATE_DIR ??
		join(import.meta.dirname, "../state");
	const now = deps.now ?? Date.now;
	const sleep =
		deps.sleep ?? ((ms: number) => new Promise<void>((r) => setTimeout(r, ms)));
	const pollMs = deps.pollMs ?? DEFAULT_POLL_MS;
	const deadline = now() + (deps.timeoutMs ?? DEFAULT_TIMEOUT_MS);
	const budget = (ms: number) => {
		const left = deadline - now();
		if (left <= 0)
			throw new ProvisionError("timeout", "workspace provisioning timed out");
		return Math.min(ms, left);
	};

	let state = readProvisionState(dir);
	if (
		state &&
		(state.workspace !== workspace || state.sessionId !== sessionId)
	) {
		throw new ProvisionError(
			"claim_conflict",
			"provision state belongs to a different workspace or session",
		);
	}
	let ws = await lookup(owner, name);
	if (!state) {
		if (ws) {
			log("provision_rejected", {
				workspace,
				session: sessionId,
				reason: "exists_without_claim",
			});
			throw new ProvisionError(
				"not_owned",
				"workspace already exists and was not created by this controller",
			);
		}
		state = {
			version: 1,
			workspace,
			sessionId,
			claim: deps.randomClaim?.() ?? randomBytes(16).toString("hex"),
			template: PROVISION_TEMPLATE,
			absentCheckedAt: new Date(now()).toISOString(),
			phase: "intent",
		};
		if (!CLAIM.test(state.claim))
			throw new ProvisionError(
				"invalid_claim",
				"claim must be 32 hex characters",
			);
		writeState(dir, state, true);
		log("provision_intent", { workspace, session: sessionId });
	}
	const claimed = () => {
		if (ws && !owns(ws, state!)) {
			log("provision_rejected", {
				workspace,
				session: sessionId,
				reason: "claim_mismatch",
			});
			throw new ProvisionError(
				"not_owned",
				"workspace does not match this controller's claim",
			);
		}
	};
	claimed();

	if (!ws) {
		if (state.workspaceId)
			throw new ProvisionError(
				"workspace_missing",
				"claimed workspace no longer exists; not recreating",
			);
		log("provision_create", {
			workspace,
			session: sessionId,
			template: PROVISION_TEMPLATE,
			templateId: CODER_TEMPLATE_ID,
		});
		budget(1);
		let created: CoderWorkspace | undefined;
		let createError = "";
		try {
			created = await create(owner, name);
		} catch (err) {
			createError = tail(err instanceof Error ? err.message : "unknown error");
		}
		if (created) {
			// The create response is authoritative for the ID; a claimed prebuild may predate our claim.
			if (
				created.ownerName !== owner ||
				created.name !== name ||
				created.templateId !== CODER_TEMPLATE_ID
			) {
				throw new ProvisionError(
					"not_owned",
					"create returned an unexpected workspace",
				);
			}
			ws = created;
		} else {
			ws = await lookup(owner, name);
			if (!ws)
				throw new ProvisionError(
					"create_failed",
					`workspace create failed: ${createError}`,
				);
			claimed();
			log("provision_create_reconciled", {
				workspace,
				session: sessionId,
				error: createError,
				status: ws.status,
			});
		}
	}
	if (!state.workspaceId) {
		state = { ...state, workspaceId: ws.id, phase: "created" };
		writeState(dir, state, false);
		log("provision_created", {
			workspace,
			session: sessionId,
			workspaceId: ws.id,
		});
	}

	let started = false;
	while (ws.status !== "running") {
		if (["deleting", "deleted"].includes(ws.status))
			throw new ProvisionError(
				"workspace_missing",
				`workspace is ${ws.status}`,
			);
		if (["stopped", "failed", "canceled"].includes(ws.status)) {
			if (started)
				throw new ProvisionError(
					"start_failed",
					`workspace is ${ws.status} after start`,
				);
			started = true;
			log("provision_start", {
				workspace,
				session: sessionId,
				status: ws.status,
			});
			// Use the remaining budget: killing the CLI would not cancel the build it queued.
			const res = await runner(
				["start", workspace, "--yes"],
				"",
				budget(Number.MAX_SAFE_INTEGER),
			);
			if (res.code !== 0)
				log("provision_start_failed", {
					workspace,
					session: sessionId,
					exit: res.code,
					error: tail(res.stderr),
				});
		} else {
			await sleep(budget(pollMs));
		}
		ws = await lookup(owner, name);
		if (!ws)
			throw new ProvisionError(
				"workspace_missing",
				"claimed workspace disappeared",
			);
		claimed();
	}

	// Startup scripts may fail independently; readiness is our own essential checks.
	let lastReason = "";
	for (;;) {
		const res = await runner(
			buildPrepareSshArgs(workspace),
			`${JSON.stringify({ op: "check" })}\n`,
			budget(CHECK_TIMEOUT_MS),
		);
		const out = parseHelperOutput(res.stdout);
		if (out?.ok) break;
		lastReason = out
			? String(out.message)
			: `ssh exit ${res.code}: ${tail(res.stderr)}`;
		if (deadline - now() <= pollMs)
			throw new ProvisionError(
				"not_ready",
				`workspace not ready: ${lastReason}`,
			);
		await sleep(pollMs);
	}
	log("provision_workspace_ready", { workspace, session: sessionId });

	const assets = deps.assets ?? loadAssets();
	const res = await runner(
		buildPrepareSshArgs(workspace),
		`${JSON.stringify({
			op: "prepare",
			workspace,
			session_id: sessionId,
			claim: state.claim,
			dockerfile: assets.dockerfile,
			entrypoint: assets.entrypoint,
		})}\n`,
		budget(PREPARE_TIMEOUT_MS),
	);
	const out: HelperResult | undefined = parseHelperOutput(res.stdout);
	if (!out)
		throw new ProvisionError(
			"ssh_failed",
			`prepare failed (exit ${res.code}): ${tail(res.stderr)}`,
		);
	if (!out.ok) {
		log("provision_prepare_failed", {
			workspace,
			session: sessionId,
			error: out.error,
			message: out.message,
			tail: typeof out.tail === "string" ? tail(out.tail) : undefined,
		});
		throw new ProvisionError(
			String(out.error ?? "prepare_failed"),
			String(out.message ?? "prepare failed"),
		);
	}
	if (typeof out.image !== "string" || !IMAGE_ID.test(out.image))
		throw new ProvisionError(
			"invalid_image",
			"prepare returned an invalid image ID",
		);
	state = { ...state, phase: "prepared", image: out.image };
	writeState(dir, state, false);
	log("provision_prepared", {
		workspace,
		session: sessionId,
		image: out.image,
		receiptReused: out.receipt_reused === true,
		codexReused: out.codex_reused === true,
	});
	return out.image;
}

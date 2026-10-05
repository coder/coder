import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
	chmodSync,
	existsSync,
	mkdirSync,
	mkdtempSync,
	readdirSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
	CoderExecutorError,
	type CoderRunner,
	parseHelperOutput,
	type RunResult,
} from "./coder.ts";
import {
	buildDockerSshArgs,
	connectDockerWorkspace,
	DOCKER_IMAGE,
	DOCKER_REMOTE_COMMAND,
	DOCKER_REMOTE_HELPER_SOURCE,
	dockerContainerName,
	dockerLabels,
	dockerWorkspaceExecutorStatus,
	stopDockerWorkspaceExecutor,
	validateDockerImage,
} from "./docker.ts";

const KEY = "sk-test-executor-key-ABCDEF123456";
const URL_OK = "https://api.openai.com/v1/agents/api/connect/rt_4h6s";
const WS = "david-fraley/codex-stage1";
const IMAGE = `debian:bookworm-slim@sha256:${"a".repeat(64)}`;
const input = {
	workspace: WS,
	environmentId: "env_one",
	remoteUrl: URL_OK,
	executorKey: KEY,
	sessionId: "sess_one",
};

function fakeRunner(result: Partial<RunResult>) {
	const calls: { args: string[]; stdin: string }[] = [];
	const runner: CoderRunner = async (args, stdin) => {
		calls.push({ args, stdin });
		return { code: 0, stdout: "", stderr: "", ...result };
	};
	return { runner, calls };
}

test("image must be digest pinned; default is the immutable executor image ID", async () => {
	assert.equal(validateDockerImage(DOCKER_IMAGE), DOCKER_IMAGE);
	assert.match(DOCKER_IMAGE, /^sha256:[0-9a-f]{64}$/);
	for (const ok of [
		IMAGE,
		`sha256:${"b".repeat(64)}`,
		`registry.example/team/img:v1@sha256:${"c".repeat(64)}`,
	])
		assert.equal(validateDockerImage(ok), ok);
	for (const bad of [
		"debian:bookworm-slim",
		"codex-webhook-executor:stage2",
		"debian@sha256:abc",
		"--privileged",
		`x@sha256:${"A".repeat(64)}`,
		42,
	]) {
		assert.throws(() => validateDockerImage(bad), /pinned/, String(bad));
	}
	const { runner, calls } = fakeRunner({
		stdout: 'STAGE2 {"ok":true,"status":"ready"}\n',
	});
	await connectDockerWorkspace(input, () => {}, { runner });
	assert.equal(JSON.parse(calls[0].stdin).image, DOCKER_IMAGE);
});

test("ssh args are fixed; name and labels are deterministic", () => {
	const args = buildDockerSshArgs(WS);
	assert.deepEqual(args.slice(0, 4), ["ssh", "--disable-autostart", WS, "--"]);
	assert.equal(args.length, 5);
	assert.equal(args[4], DOCKER_REMOTE_COMMAND);
	assert.match(
		args[4],
		/^python3 -c "import base64;exec\(base64\.b64decode\('[A-Za-z0-9+/=]+'\)\)"$/,
	);
	assert.equal(
		dockerContainerName("sess_one"),
		dockerContainerName("sess_one"),
	);
	assert.notEqual(
		dockerContainerName("sess_one"),
		dockerContainerName("sess_two"),
	);
	assert.match(dockerContainerName("sess_one"), /^codex-stage2-[0-9a-f]{24}$/);
	assert.deepEqual(dockerLabels(WS, "s", "e", URL_OK), {
		"dev.coder.codex-stage2.prototype": "docker-executor-v1",
		"dev.coder.codex-stage2.owner": WS,
		"dev.coder.codex-stage2.session": "s",
		"dev.coder.codex-stage2.environment": "e",
		"dev.coder.codex-stage2.remote-url": URL_OK,
	});
	const r = spawnSync(
		"python3",
		["-c", 'import sys;compile(sys.stdin.read(),"h","exec")'],
		{ input: DOCKER_REMOTE_HELPER_SOURCE },
	);
	assert.equal(r.status, 0, r.stderr.toString());
});

test("connect sends key only on stdin with pinned image and redacts logs", async () => {
	const { runner, calls } = fakeRunner({
		stdout: `STAGE2 {"ok":true,"status":"ready","reused":false,"container_id":"c1","echo":"${KEY}"}\n`,
		stderr: KEY,
	});
	const entries: unknown[] = [];
	await connectDockerWorkspace(
		input,
		(kind, data) => entries.push({ kind, data }),
		{ runner, image: IMAGE },
	);
	assert.ok(!calls[0].args.join(" ").includes(KEY));
	assert.deepEqual(JSON.parse(calls[0].stdin), {
		op: "connect",
		workspace: WS,
		session_id: "sess_one",
		environment_id: "env_one",
		remote_url: URL_OK,
		image: IMAGE,
		executor_key: KEY,
	});
	assert.equal(calls[0].stdin.split("\n").length, 2);
	assert.ok(!JSON.stringify(entries).includes(KEY));
});

test("helper errors surface as coded errors without the key", async () => {
	const busy = fakeRunner({
		code: 3,
		stdout: `STAGE2 {"ok":false,"error":"busy","message":"m ${KEY}","owner_session":"sess_two"}\n`,
	});
	await assert.rejects(
		connectDockerWorkspace(input, () => {}, {
			runner: busy.runner,
			image: IMAGE,
		}),
		(e: unknown) =>
			e instanceof CoderExecutorError &&
			e.code === "busy" &&
			e.details.owner_session === "sess_two" &&
			!e.message.includes(KEY),
	);
	const ssh = fakeRunner({ code: 1, stderr: `boom ${KEY}\n` });
	await assert.rejects(
		connectDockerWorkspace(input, () => {}, {
			runner: ssh.runner,
			image: IMAGE,
		}),
		(e: unknown) =>
			e instanceof CoderExecutorError &&
			e.code === "ssh_failed" &&
			!e.message.includes(KEY),
	);
	const stop = fakeRunner({
		stdout: 'STAGE2 {"ok":true,"status":"stopped"}\n',
	});
	await stopDockerWorkspaceExecutor(WS, "sess_one", { runner: stop.runner });
	assert.deepEqual(JSON.parse(stop.calls[0].stdin), {
		op: "stop",
		workspace: WS,
		session_id: "sess_one",
	});
	const st = fakeRunner({
		stdout:
			'STAGE2 {"ok":true,"status":"ready","alive":true,"session_id":"s","environment_id":"e","container_id":"c"}\n',
	});
	assert.deepEqual(
		await dockerWorkspaceExecutorStatus(WS, { runner: st.runner }),
		{
			status: "ready",
			alive: true,
			sessionId: "s",
			environmentId: "e",
			containerId: "c",
		},
	);
});

// Fake docker CLI: containers live in a JSON file; `start --attach` becomes the
// container's main process until it is signalled by stop/kill.
const FAKE_DOCKER = String.raw`#!/usr/bin/env python3
import fcntl, json, os, signal, subprocess, sys, time
ROOT = os.environ.get('FAKE_ROOT') or os.path.dirname(os.path.abspath(__file__))
DB = os.path.join(ROOT, 'containers.json')
VALUE_FLAGS = {'--name', '--label', '--pull', '--user', '--cap-drop', '--security-opt', '--network', '--ipc', '--pids-limit',
               '--hostname', '--restart', '--stop-timeout', '--log-driver', '--tmpfs', '--mount', '--workdir', '--entrypoint'}
BOOL_FLAGS = {'--interactive', '--init', '--read-only'}
IMAGE = open(os.path.join(ROOT, 'image')).read().strip()
open(os.path.join(ROOT, 'calls.log'), 'a').write(json.dumps(sys.argv[1:]) + '\n')

class Db:
    def __enter__(self):
        self.fd = open(os.path.join(ROOT, 'db.lock'), 'w')
        fcntl.flock(self.fd, fcntl.LOCK_EX)
        self.data = json.load(open(DB)) if os.path.exists(DB) else {}
        return self.data
    def __exit__(self, *a):
        json.dump(self.data, open(DB + '.tmp', 'w'))
        os.replace(DB + '.tmp', DB)
        self.fd.close()

def find(db, ref):
    for c in db.values():
        if c['Id'] == ref or c['Name'] == '/' + ref:
            return c
    return None

def set_state(ref, status, code=0, pid=0):
    with Db() as db:
        c = find(db, ref)
        c['State'] = {'Status': status, 'Running': status == 'running', 'ExitCode': code, 'Pid': pid}

def wait_not_running(ref, t):
    deadline = time.time() + t
    while time.time() < deadline:
        with Db() as db:
            if find(db, ref)['State']['Status'] != 'running':
                return True
        time.sleep(0.05)
    return False

a = sys.argv[1:]
if a[:2] == ['image', 'inspect']:
    if a[-1] != IMAGE:
        sys.exit(1)
    print(open(os.path.join(ROOT, 'entrypoint')).read())
    sys.exit(0)
if a[0] == 'create':
    opts, i = {}, 1
    while a[i].startswith('--'):
        if a[i] in BOOL_FLAGS:
            opts.setdefault(a[i], []).append(True); i += 1
        else:
            assert a[i] in VALUE_FLAGS, a[i]
            opts.setdefault(a[i], []).append(a[i + 1]); i += 2
    cid = os.urandom(32).hex()
    labels = dict(l.split('=', 1) for l in opts.get('--label', []))
    labels['org.example.from-image'] = 'x'
    with Db() as db:
        db[cid] = {'Id': cid, 'Name': '/' + opts['--name'][0], 'Config': {'Labels': labels, 'Image': a[i], 'Cmd': a[i + 1:], 'Env': []},
                   'State': {'Status': 'created', 'Running': False, 'ExitCode': 0, 'Pid': 0}, 'Argv': a}
    print(cid)
elif a[:2] == ['container', 'inspect']:
    with Db() as db:
        c = find(db, a[2])
    if not c:
        sys.stderr.write('Error: No such container: %s\n' % a[2]); sys.exit(1)
    print(json.dumps([c]))
elif a[0] == 'ps':
    k, v = a[a.index('--filter') + 1][len('label='):].split('=', 1)
    with Db() as db:
        for c in db.values():
            if c['Config']['Labels'].get(k) == v:
                print(c['Id'])
elif a[0] == 'start':
    cid = a[-1]
    set_state(cid, 'running', pid=os.getpid())
    payload = json.loads(sys.stdin.readline())
    open(os.path.join(ROOT, 'stdin_keys.json'), 'w').write(json.dumps(sorted(payload)))
    child = subprocess.Popen(['sleep', '300'])
    open(os.path.join(ROOT, 'child.' + cid), 'w').write(str(child.pid))
    def term(*_):
        child.kill()
        set_state(cid, 'exited', 143)
        os._exit(143)
    signal.signal(signal.SIGTERM, term)
    if not os.path.exists(os.path.join(ROOT, 'no_ready')):
        print('INFO Noise executor connected to rendezvous noise_outcome="ok" key=' + payload['executor_key'], flush=True)
    while True:
        time.sleep(1)
elif a[0] in ('stop', 'kill'):
    cid = a[-1]
    with Db() as db:
        c = find(db, cid)
    if c['State']['Status'] == 'running':
        sig = signal.SIGTERM if a[0] == 'stop' else signal.SIGKILL
        try:
            os.kill(c['State']['Pid'], sig)
        except OSError:
            pass
        if not wait_not_running(cid, 3):
            set_state(cid, 'exited', 137)
elif a[0] == 'rm':
    with Db() as db:
        c = find(db, a[1])
        if c['State']['Status'] == 'running':
            sys.exit(1)
        del db[c['Id']]
else:
    sys.exit(2)
`;

test("helper manages only owned containers with reuse, busy, timeout cleanup, and verified stop", () => {
	const root = mkdtempSync(join(tmpdir(), "docker-helper-"));
	try {
		const control = join(root, "control");
		const demo = join(root, "demo");
		mkdirSync(demo);
		const bin = join(root, "codex");
		writeFileSync(bin, "");
		chmodSync(bin, 0o755);
		const fake = join(root, "docker");
		writeFileSync(fake, FAKE_DOCKER);
		chmodSync(fake, 0o755);
		writeFileSync(join(root, "image"), IMAGE);
		writeFileSync(join(root, "entrypoint"), '["python3","/bin/sh"]');
		// An unrelated running container on the same daemon must never be touched.
		const unknownId = "f".repeat(64);
		const unknown = {
			Id: unknownId,
			Name: "/other",
			Config: {
				Labels: { "dev.coder.codex-stage2.prototype": "something-else" },
				Image: "x",
				Cmd: [],
			},
			State: { Status: "running", Running: true, ExitCode: 0, Pid: 999999 },
		};
		writeFileSync(
			join(root, "containers.json"),
			JSON.stringify({ [unknownId]: unknown }),
		);
		const src = DOCKER_REMOTE_HELPER_SOURCE.replace(
			"'/home/coder/.codex-stage2-docker'",
			JSON.stringify(control),
		)
			.replace("DOCKER = '/usr/bin/docker'", `DOCKER = ${JSON.stringify(fake)}`)
			.replace(/BINARY = '[^']+'/, `BINARY = ${JSON.stringify(bin)}`)
			.replace("DEMO = '/home/coder/demo'", `DEMO = ${JSON.stringify(demo)}`)
			.replace("READY_TIMEOUT = 90", "READY_TIMEOUT = 3")
			.replace("STOP_TIMEOUT = 10", "STOP_TIMEOUT = 2");
		const run = (payload: Record<string, unknown>) => {
			const r = spawnSync("python3", ["-c", src], {
				input: `${JSON.stringify(payload)}\n`,
				timeout: 60_000,
				env: { PATH: process.env.PATH, FAKE_ROOT: root },
			});
			const out = parseHelperOutput(r.stdout.toString());
			assert.ok(out, r.stderr.toString());
			return out;
		};
		const db = () =>
			JSON.parse(readFileSync(join(root, "containers.json"), "utf8"));
		const conn = {
			op: "connect",
			workspace: WS,
			session_id: "sess_one",
			environment_id: "env_one",
			remote_url: URL_OK,
			image: IMAGE,
			executor_key: KEY,
		};

		assert.equal(run({ op: "status", workspace: WS }).status, "none");
		assert.equal(
			run({ ...conn, image: "debian:bookworm-slim" }).error,
			"invalid_input",
		);
		assert.equal(
			run({ ...conn, image: `sha256:${"e".repeat(64)}` }).error,
			"not_prepared",
		);
		const wrongEntry = run(conn);
		assert.equal(wrongEntry.error, "not_prepared");
		assert.match(String(wrongEntry.message), /entrypoint/);
		writeFileSync(
			join(root, "entrypoint"),
			'["python3","/usr/local/bin/executor-entrypoint.py"]',
		);
		const first = run(conn);
		assert.equal(first.ok, true, JSON.stringify(first));
		assert.equal(first.reused, false);
		const cid = first.container_id as string;
		assert.match(cid, /^[0-9a-f]{64}$/);
		assert.equal(
			JSON.parse(readFileSync(join(control, "executor.json"), "utf8"))
				.container_id,
			cid,
		);
		assert.deepEqual(
			JSON.parse(readFileSync(join(root, "stdin_keys.json"), "utf8")),
			["environment_id", "executor_key", "remote_url"],
		);

		const c = db()[cid];
		assert.equal(c.Name, `/${dockerContainerName("sess_one")}`);
		const { "org.example.from-image": _inherited, ...ownLabels } =
			c.Config.Labels;
		assert.deepEqual(
			ownLabels,
			dockerLabels(WS, "sess_one", "env_one", URL_OK),
		);
		assert.deepEqual(c.Config.Cmd, []);
		// The image ENTRYPOINT runs as-is: the image is the last create argument, with no command or shell after it.
		assert.equal(c.Argv.at(-1), IMAGE);
		assert.ok(
			!c.Argv.some((a: string) => /(^|\/)(ba)?sh$|^-c$/.test(a)),
			"no shell in create args",
		);
		const argv: string[] = c.Argv;
		const flag = (f: string) =>
			argv.flatMap((a, i) => (a === f ? [argv[i + 1]] : []));
		assert.deepEqual(flag("--user"), ["1000:1000"]);
		assert.deepEqual(flag("--cap-drop"), ["ALL"]);
		assert.deepEqual(flag("--security-opt"), ["no-new-privileges"]);
		assert.deepEqual(flag("--network"), ["bridge"]);
		assert.deepEqual(flag("--pull"), ["never"]);
		assert.ok(
			argv.includes("--read-only") &&
				argv.includes("--init") &&
				argv.includes("--interactive"),
		);
		assert.deepEqual(flag("--mount"), [
			`type=bind,source=${bin},target=/usr/local/bin/codex,readonly`,
			`type=bind,source=${demo},target=/home/coder/demo`,
		]);
		assert.deepEqual(flag("--tmpfs"), [
			"/tmp:rw,nosuid,nodev,size=512m,mode=1777",
		]);
		for (const banned of [
			"--entrypoint",
			"--privileged",
			"-v",
			"--volume",
			"--env",
			"-e",
			"--env-file",
			"--cap-add",
			"--pid",
			"--userns",
		])
			assert.ok(!argv.includes(banned), banned);
		assert.ok(
			!argv.join(" ").includes("docker.sock") && !argv.includes("host"),
		);

		const again = run(conn);
		assert.equal(again.reused, true, JSON.stringify(again));
		assert.equal(again.container_id, cid);
		assert.equal(run({ ...conn, session_id: "sess_two" }).error, "busy");
		assert.equal(run({ ...conn, environment_id: "env_two" }).error, "conflict");
		assert.equal(
			run({ ...conn, remote_url: `${URL_OK}/other` }).error,
			"conflict",
		);
		assert.deepEqual(run({ op: "status", workspace: WS }), {
			ok: true,
			status: "ready",
			alive: true,
			session_id: "sess_one",
			environment_id: "env_one",
			container_id: cid,
		});
		assert.equal(
			run({ op: "stop", workspace: WS, session_id: "sess_two" }).error,
			"not_owner",
		);
		assert.equal(
			run({ op: "stop", workspace: "other/ws", session_id: "sess_one" }).error,
			"not_owner",
		);

		const child = Number(readFileSync(join(root, `child.${cid}`), "utf8"));
		const stopped = run({ op: "stop", workspace: WS, session_id: "sess_one" });
		assert.equal(stopped.status, "stopped", JSON.stringify(stopped));
		assert.equal(
			db()[cid],
			undefined,
			"owned container removed after verified exit",
		);
		const gone = (pid: number) =>
			!existsSync(`/proc/${pid}`) ||
			/\) [ZX] /.test(readFileSync(`/proc/${pid}/stat`, "utf8"));
		assert.ok(gone(child));
		assert.equal(
			run({ op: "stop", workspace: WS, session_id: "sess_one" }).status,
			"not_running",
		);

		// Startup timeout stops the exact container and its children but keeps it as evidence.
		writeFileSync(join(root, "no_ready"), "");
		const timedOut = run({ ...conn, session_id: "sess_three" });
		assert.equal(timedOut.error, "ready_timeout", JSON.stringify(timedOut));
		assert.equal(timedOut.stopped, true);
		const evidence = db()[timedOut.container_id as string];
		assert.equal(evidence.State.Status, "exited");
		assert.ok(
			gone(
				Number(
					readFileSync(join(root, `child.${timedOut.container_id}`), "utf8"),
				),
			),
		);
		assert.equal(run({ op: "status", workspace: WS }).alive, false);
		// A different session may start once nothing is active.
		rmSync(join(root, "no_ready"));
		const next = run({ ...conn, session_id: "sess_four" });
		assert.equal(next.ok, true, JSON.stringify(next));
		assert.equal(
			run({ op: "stop", workspace: WS, session_id: "sess_four" }).status,
			"stopped",
		);
		assert.equal(
			run({ op: "stop", workspace: WS, session_id: "sess_three" }).status,
			"stopped",
		);

		assert.deepEqual(db()[unknownId], unknown, "unrelated container untouched");
		const calls = readFileSync(join(root, "calls.log"), "utf8");
		assert.ok(!calls.includes(unknownId), "unknown container never addressed");
		assert.ok(!calls.includes(KEY), "key never in docker args");
		assert.ok(
			!readFileSync(join(root, "containers.json"), "utf8").includes(KEY),
			"key not in container config",
		);
		for (const f of readdirSync(control))
			assert.ok(
				!readFileSync(join(control, f), "utf8").includes(KEY),
				`${f} has no key`,
			);
		assert.match(
			readFileSync(join(control, "executor.log"), "utf8"),
			/key=\[REDACTED\]/,
		);
	} finally {
		spawnSync("pkill", ["-f", root]);
		rmSync(root, { recursive: true, force: true });
	}
});

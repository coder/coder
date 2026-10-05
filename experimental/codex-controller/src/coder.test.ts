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
	buildSshArgs,
	CoderExecutorError,
	type CoderRunner,
	coderEnv,
	connectWorkspace,
	parseHelperOutput,
	REMOTE_COMMAND,
	REMOTE_HELPER_SOURCE,
	type RunResult,
	redact,
	stopWorkspaceExecutor,
	validateRemoteUrl,
	validateWorkspace,
	workspaceExecutorStatus,
} from "./coder.ts";

const KEY = "sk-test-executor-key-ABCDEF123456";
const URL_OK = "https://api.openai.com/v1/agents/api/connect/rt_4h6s";
const input = {
	workspace: "david-fraley/codex-stage1",
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

function recorder() {
	const entries: { kind: string; data: Record<string, unknown> }[] = [];
	return {
		entries,
		log: (kind: string, data: Record<string, unknown>) => {
			entries.push({ kind, data });
		},
	};
}

test("workspace must be a safe owner/name reference", () => {
	for (const ok of ["david-fraley/codex-stage1", "a/b", "Owner1/ws-2"])
		assert.equal(validateWorkspace(ok), ok);
	for (const bad of [
		"codex-stage1",
		"-x/ws",
		"a/-ws",
		"a/b/c",
		"a/b.agent",
		"a/b;rm",
		"a/ b",
		"a--b/ws",
		`a/${"x".repeat(33)}`,
		"",
		42,
	]) {
		assert.throws(() => validateWorkspace(bad), /owner\/name/, String(bad));
	}
});

test("remote URL must be an OpenAI executor connect URL", () => {
	for (const ok of [
		URL_OK,
		"https://api.openai.com/v1/agents/api/connect/rt_9v4n/cloud/environment/ccarenv_b64_Y2Nh",
	])
		assert.equal(validateRemoteUrl(ok), ok);
	for (const bad of [
		"http://api.openai.com/v1/agents/api/connect/rt",
		"https://api.openai.com.evil.test/v1/agents/api/connect/rt",
		"https://user@api.openai.com/v1/agents/api/connect/rt",
		"https://api.openai.com:8443/v1/agents/api/connect/rt",
		"https://api.openai.com/v1/agents/api/connect/",
		"https://api.openai.com/v1/agents/api/connect/rt?x=1",
		"https://api.openai.com/v1/agents/api/connect/rt#x",
		"https://api.openai.com/v1/agents/api/connect/../../files",
		"https://api.openai.com/v1/agents/api/connect/%2e%2e",
		"https://api.openai.com/v1/other/connect/rt",
		"https://evil.test/https://api.openai.com/v1/agents/api/connect/rt",
	])
		assert.throws(() => validateRemoteUrl(bad), /untrusted/, bad);
});

test("ssh args are a fixed array with no caller data beyond the workspace", () => {
	const args = buildSshArgs("david-fraley/codex-stage1");
	assert.deepEqual(args.slice(0, 4), [
		"ssh",
		"--disable-autostart",
		"david-fraley/codex-stage1",
		"--",
	]);
	assert.equal(args.length, 5);
	assert.match(
		args[4],
		/^python3 -c "import base64;exec\(base64\.b64decode\('[A-Za-z0-9+/=]+'\)\)"$/,
	);
	assert.equal(args[4], REMOTE_COMMAND);
	assert.throws(() => buildSshArgs("--help/x"));
});

test("helper source compiles and ignores credential-bearing env for coder", () => {
	const r = spawnSync(
		"python3",
		["-c", 'import sys;compile(sys.stdin.read(),"helper","exec")'],
		{ input: REMOTE_HELPER_SOURCE },
	);
	assert.equal(r.status, 0, r.stderr.toString());
	const env = coderEnv({
		PATH: "/bin",
		HOME: "/h",
		CODER_SESSION_TOKEN: "t",
		OPENAI_API_KEY: "x",
		OPENAI_EXECUTOR_API_KEY: "y",
	});
	assert.deepEqual(Object.keys(env).sort(), [
		"CODER_SESSION_TOKEN",
		"HOME",
		"PATH",
	]);
});

test("connect sends key only on stdin and never logs it, even if echoed", async () => {
	const { runner, calls } = fakeRunner({
		stdout: `noise\nSTAGE2 {"ok":true,"status":"ready","reused":false,"executor_pid":7,"echo":"${KEY}"}\n`,
		stderr: `warn ${KEY}`,
	});
	const { entries, log } = recorder();
	await connectWorkspace(input, log, { runner });
	assert.equal(calls.length, 1);
	assert.ok(!calls[0].args.join(" ").includes(KEY));
	const payload = JSON.parse(calls[0].stdin);
	assert.deepEqual(payload, {
		op: "connect",
		session_id: "sess_one",
		environment_id: "env_one",
		remote_url: URL_OK,
		executor_key: KEY,
	});
	assert.ok(
		calls[0].stdin.endsWith("\n") && calls[0].stdin.split("\n").length === 2,
	);
	assert.ok(!JSON.stringify(entries).includes(KEY));
	assert.equal(entries.at(-1)?.kind, "coder_executor_ready");
});

test("connect rejects bad input before running coder", async () => {
	const { runner, calls } = fakeRunner({});
	const { log } = recorder();
	await assert.rejects(
		connectWorkspace(
			{ ...input, remoteUrl: "https://evil.test/v1/agents/api/connect/x" },
			log,
			{ runner },
		),
		/untrusted/,
	);
	await assert.rejects(
		connectWorkspace({ ...input, workspace: "x" }, log, { runner }),
		/owner\/name/,
	);
	await assert.rejects(
		connectWorkspace({ ...input, sessionId: "sess one" }, log, { runner }),
		/session id/,
	);
	await assert.rejects(
		connectWorkspace({ ...input, executorKey: "a b\nccccccc" }, log, {
			runner,
		}),
		(e) => !String(e).includes("ccccccc"),
	);
	assert.equal(calls.length, 0);
});

test("busy helper result surfaces as a coded error without the key", async () => {
	const { runner } = fakeRunner({
		code: 3,
		stdout:
			'STAGE2 {"ok":false,"error":"busy","message":"executor is owned by another session","owner_session":"sess_two"}\n',
	});
	const { entries, log } = recorder();
	await assert.rejects(
		connectWorkspace(input, log, { runner }),
		(e: unknown) =>
			e instanceof CoderExecutorError &&
			e.code === "busy" &&
			e.details.owner_session === "sess_two",
	);
	assert.equal(entries.at(-1)?.kind, "coder_executor_connect_failed");
});

test("missing helper result reports redacted ssh failure", async () => {
	const { runner } = fakeRunner({
		code: 1,
		stderr: `error: workspace must be started ${KEY}\n`,
	});
	await assert.rejects(
		connectWorkspace(input, () => {}, { runner }),
		(e: unknown) =>
			e instanceof CoderExecutorError &&
			e.code === "ssh_failed" &&
			!e.message.includes(KEY) &&
			e.message.includes("must be started"),
	);
});

test("stop and status send no key and parse results", async () => {
	const stop = fakeRunner({
		stdout: 'STAGE2 {"ok":true,"status":"stopped","leftovers":[]}\n',
	});
	await stopWorkspaceExecutor("david-fraley/codex-stage1", "sess_one", {
		runner: stop.runner,
	});
	assert.deepEqual(JSON.parse(stop.calls[0].stdin), {
		op: "stop",
		session_id: "sess_one",
	});
	const notOwner = fakeRunner({
		code: 3,
		stdout: 'STAGE2 {"ok":false,"error":"not_owner","message":"owned"}\n',
	});
	await assert.rejects(
		stopWorkspaceExecutor("david-fraley/codex-stage1", "sess_one", {
			runner: notOwner.runner,
		}),
		(e: unknown) => e instanceof CoderExecutorError && e.code === "not_owner",
	);
	const status = fakeRunner({
		stdout:
			'STAGE2 {"ok":true,"status":"ready","alive":true,"session_id":"sess_one","environment_id":"env_one"}\n',
	});
	assert.deepEqual(
		await workspaceExecutorStatus("david-fraley/codex-stage1", {
			runner: status.runner,
		}),
		{
			status: "ready",
			alive: true,
			sessionId: "sess_one",
			environmentId: "env_one",
		},
	);
});

test("parse and redact helpers", () => {
	assert.equal(parseHelperOutput("x\nSTAGE2 {bad\n"), undefined);
	assert.equal(
		parseHelperOutput(
			'STAGE2 {"ok":true,"n":1}\nSTAGE2 {"ok":false,"error":"e"}\n',
		)?.error,
		"e",
	);
	assert.equal(redact(`a${KEY}b${KEY}`, [KEY]), "a[REDACTED]b[REDACTED]");
});

function assertBoundary(
	argv: string[],
	p: { bin: string; demo: string; home: string; resolv: string },
) {
	const sep = argv.indexOf("--");
	assert.deepEqual(argv.slice(sep + 1), [
		p.bin,
		"exec-server",
		"--remote",
		URL_OK,
		"--environment-id",
		"env_one",
	]);
	const opts = argv.slice(0, sep);
	for (const f of [
		"--unshare-user",
		"--unshare-pid",
		"--unshare-ipc",
		"--die-with-parent",
		"--new-session",
	])
		assert.ok(opts.includes(f), f);
	assert.equal(opts[opts.indexOf("--cap-drop") + 1], "ALL");
	const ops: { op: string; args: string[] }[] = [];
	const arity: Record<string, number> = {
		"--bind": 2,
		"--ro-bind": 2,
		"--ro-bind-try": 2,
		"--symlink": 2,
		"--tmpfs": 1,
		"--dir": 1,
		"--proc": 1,
		"--dev": 1,
		"--chdir": 1,
		"--hostname": 1,
		"--cap-drop": 1,
	};
	for (let i = 0; i < opts.length; i++) {
		const n = arity[opts[i]];
		if (n === undefined) {
			assert.match(opts[i], /^--unshare-|^--die-with-parent$|^--new-session$/);
			continue;
		}
		ops.push({ op: opts[i], args: opts.slice(i + 1, i + 1 + n) });
		i += n;
	}
	const at = (op: string, target: string) =>
		ops.findIndex((o) => o.op === op && o.args.at(-1) === target);
	// Only the demo directory and Codex state are writable.
	assert.deepEqual(
		ops.filter((o) => o.op === "--bind").map((o) => o.args),
		[
			[p.demo, p.demo],
			[p.home, p.home],
		],
	);
	const systemDirs = new Set([
		"/usr",
		"/bin",
		"/sbin",
		"/lib",
		"/lib32",
		"/lib64",
		"/libx32",
	]);
	for (const o of ops.filter(
		(o) => o.op === "--ro-bind" || o.op === "--ro-bind-try",
	)) {
		const [src, dst] = o.args;
		const allowed =
			systemDirs.has(src) ||
			(src.startsWith("/etc/") && src !== "/etc/" && src === dst) ||
			src === p.bin ||
			(src === p.resolv && dst === "/etc/resolv.conf");
		assert.ok(allowed, `unexpected read-only mount ${src}`);
		assert.ok(
			![
				"/",
				"/etc",
				"/home",
				"/run",
				"/var",
				"/opt",
				"/root",
				"/sys",
				"/tmp",
				"/etc/ssl",
				"/etc/ssl/private",
			].includes(src),
			src,
		);
	}
	for (const t of ["/run", "/tmp", "/var", "/home"])
		assert.ok(at("--tmpfs", t) >= 0, `tmpfs ${t}`);
	assert.ok(at("--proc", "/proc") >= 0 && at("--dev", "/dev") >= 0);
	// Masks are applied before the allowlisted binds so the binds stay visible.
	const lastMask = Math.max(at("--tmpfs", "/home"), at("--tmpfs", "/tmp"));
	for (const target of [p.demo, p.home, p.bin])
		assert.ok(
			ops.findIndex((o) => o.args.at(-1) === target) > lastMask,
			target,
		);
}

// Runs the real helper locally against temp paths with a fake bwrap/executor.
// The fake prints the rendezvous line, spawns a child, and ignores SIGTERM so
// stop must fall back to SIGKILL and verify descendants are gone.
test("helper supervises detached executor with ownership, reuse, and verified cleanup", () => {
	const root = mkdtempSync(join(tmpdir(), "coder-helper-"));
	try {
		const control = join(root, "control");
		const home = join(root, "codex-home");
		const demo = join(root, "demo");
		mkdirSync(demo);
		const hostResolv = join(root, "host-resolv.conf");
		writeFileSync(
			hostResolv,
			"search svc.cluster.local cluster.local\nnameserver 10.0.0.10\nnameserver bad;value\noptions ndots:5\n",
		);
		const bin = join(root, "codex");
		writeFileSync(bin, "");
		chmodSync(bin, 0o755);
		const childPidFile = join(root, "child.pid");
		const argvFile = join(root, "argv.json");
		const fake = join(root, "bwrap");
		writeFileSync(
			fake,
			`#!/usr/bin/env python3
import json, os, signal, subprocess, sys, time
signal.signal(signal.SIGTERM, signal.SIG_IGN)
assert os.environ.get('CODEX_API_KEY') and not any(os.environ['CODEX_API_KEY'] in a for a in sys.argv)
assert sorted(set(os.environ) - {'LC_CTYPE'}) == ['CODEX_API_KEY', 'CODEX_HOME', 'HOME', 'PATH', 'RUST_LOG']  # LC_CTYPE: set by this fake's own Python locale coercion
open(${JSON.stringify(argvFile)}, 'w').write(json.dumps(sys.argv[1:]))
c = subprocess.Popen(['sleep', '300'])
open(${JSON.stringify(childPidFile)}, 'w').write(str(c.pid))
print('INFO Noise executor registration completed', flush=True)
print('INFO Noise executor connected to rendezvous noise_outcome="ok"', flush=True)
time.sleep(300)
`,
		);
		chmodSync(fake, 0o755);
		const src = REMOTE_HELPER_SOURCE.replace(
			"'/home/coder/.codex-stage2-control'",
			JSON.stringify(control),
		)
			.replace(
				"CODEX_HOME = '/home/coder/.codex-executor'",
				`CODEX_HOME = ${JSON.stringify(home)}`,
			)
			.replace(
				"HOST_RESOLV = '/etc/resolv.conf'",
				`HOST_RESOLV = ${JSON.stringify(hostResolv)}`,
			)
			.replace(
				"WORKDIR = '/home/coder/demo'",
				`WORKDIR = ${JSON.stringify(demo)}`,
			)
			.replace("BWRAP = '/usr/bin/bwrap'", `BWRAP = ${JSON.stringify(fake)}`)
			.replace(/BINARY = '[^']+'/, `BINARY = ${JSON.stringify(bin)}`)
			.replace("STOP_TIMEOUT = 15", "STOP_TIMEOUT = 1");
		assert.ok(!src.includes("'/home/coder/"), "all workspace paths redirected");
		const run = (payload: Record<string, unknown>) => {
			const r = spawnSync("python3", ["-c", src], {
				input: `${JSON.stringify(payload)}\n`,
				timeout: 60_000,
			});
			const out = parseHelperOutput(r.stdout.toString());
			assert.ok(out, r.stderr.toString());
			return out;
		};
		const conn = {
			op: "connect",
			session_id: "sess_one",
			environment_id: "env_one",
			remote_url: URL_OK,
			executor_key: KEY,
		};

		assert.equal(run({ op: "status" }).status, "none");
		assert.equal(
			run({ ...conn, remote_url: "https://evil.test/x" }).error,
			"invalid_input",
		);
		const first = run(conn);
		assert.equal(first.ok, true, JSON.stringify(first));
		assert.equal(first.reused, false);
		// The helper and SSH call have returned; the executor keeps running detached.
		const again = run(conn);
		assert.equal(again.reused, true);
		assert.equal(again.executor_pid, first.executor_pid);
		assertBoundary(JSON.parse(readFileSync(argvFile, "utf8")), {
			bin,
			demo,
			home,
			resolv: join(control, "resolv.conf"),
		});
		assert.equal(
			readFileSync(join(control, "resolv.conf"), "utf8"),
			"nameserver 10.0.0.10\noptions ndots:1 timeout:3 attempts:2\n",
		);
		assert.equal(run({ ...conn, session_id: "sess_two" }).error, "busy");
		assert.equal(run({ ...conn, environment_id: "env_two" }).error, "conflict");
		assert.deepEqual(run({ op: "status" }), {
			ok: true,
			status: "ready",
			alive: true,
			session_id: "sess_one",
			environment_id: "env_one",
		});
		assert.equal(
			run({ op: "stop", session_id: "sess_two" }).error,
			"not_owner",
		);

		const childPid = Number(readFileSync(childPidFile, "utf8"));
		assert.ok(existsSync(`/proc/${childPid}`));
		const stopped = run({ op: "stop", session_id: "sess_one" });
		assert.equal(stopped.status, "stopped", JSON.stringify(stopped));
		const gone = (pid: number) =>
			!existsSync(`/proc/${pid}`) ||
			/\) [ZX] /.test(readFileSync(`/proc/${pid}/stat`, "utf8"));
		assert.ok(gone(childPid), "executor descendant was killed");
		assert.ok(gone(first.executor_pid as number), "executor was killed");
		assert.equal(
			run({ op: "stop", session_id: "sess_one" }).status,
			"not_running",
		);
		assert.equal(run({ op: "status" }).status, "stopped");

		// Another session may take over only after the previous run is gone.
		const next = run({ ...conn, session_id: "sess_two" });
		assert.equal(next.ok, true);
		assert.equal(run({ op: "stop", session_id: "sess_two" }).status, "stopped");

		for (const f of readdirSync(control))
			assert.ok(
				!readFileSync(join(control, f), "utf8").includes(KEY),
				`${f} has no key`,
			);
	} finally {
		// Best-effort cleanup of any fake processes if an assertion failed midway.
		spawnSync("pkill", ["-f", root]);
		rmSync(root, { recursive: true, force: true });
	}
});

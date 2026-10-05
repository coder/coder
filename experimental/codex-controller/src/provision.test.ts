import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
	chmodSync,
	existsSync,
	mkdirSync,
	mkdtempSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
	type CoderRunner,
	parseHelperOutput,
	type RunResult,
} from "./coder.ts";
import {
	buildCreateBody,
	buildPrepareSshArgs,
	CODER_TEMPLATE_ID,
	type CoderWorkspace,
	createWorkspaceViaApi,
	fetchWorkspace,
	PREPARE_HELPER_SOURCE,
	PREPARE_REMOTE_COMMAND,
	type ProvisionDeps,
	ProvisionError,
	prepareOnDemandWorkspace,
	provisionEnv,
	readProvisionState,
} from "./provision.ts";

const WS = "david-fraley/codex-worker";
const SESSION = "sess_one";
const IMAGE = `sha256:${"ab".repeat(32)}`;
const ID = "11111111-2222-3333-4444-555555555555";
const T0 = Date.parse("2026-10-05T12:00:00Z");

type CreateMode = "ok" | "throw-made" | "throw-none" | "prebuild" | "foreign";

function harness(
	opts: {
		existing?: Partial<CoderWorkspace> | null;
		create?: CreateMode;
		checks?: boolean[];
		image?: unknown;
	} = {},
) {
	const dir = mkdtempSync(join(tmpdir(), "provision-"));
	let clock = T0;
	const mk = (over: Partial<CoderWorkspace> = {}): CoderWorkspace => ({
		id: ID,
		ownerName: "david-fraley",
		name: "codex-worker",
		templateId: CODER_TEMPLATE_ID,
		templateName: "coder",
		createdAt: new Date(clock).toISOString(),
		status: "running",
		...over,
	});
	const world = {
		ws:
			opts.existing === undefined || opts.existing === null
				? null
				: mk(opts.existing),
	};
	const checks = [...(opts.checks ?? [true])];
	const calls: { args: string[]; stdin: string }[] = [];
	const logs: { kind: string; data: Record<string, unknown> }[] = [];
	const runner: CoderRunner = async (args, stdin): Promise<RunResult> => {
		calls.push({ args, stdin });
		clock += 1000;
		if (args[0] === "start") {
			if (world.ws) world.ws = { ...world.ws, status: "running" };
			return { code: 0, stdout: "", stderr: "" };
		}
		const op = JSON.parse(stdin).op;
		if (op === "check") {
			const ok = checks.length > 1 ? checks.shift() : checks[0];
			return {
				code: ok ? 0 : 3,
				stdout: ok
					? 'STAGE2 {"ok":true}\n'
					: 'STAGE2 {"ok":false,"error":"not_ready","message":"workspace is missing docker"}\n',
				stderr: "",
			};
		}
		return {
			code: 0,
			stdout:
				"STAGE2 " +
				JSON.stringify({ ok: true, image: opts.image ?? IMAGE }) +
				"\n",
			stderr: "",
		};
	};
	const deps: ProvisionDeps = {
		runner,
		stateDir: dir,
		now: () => clock,
		sleep: async (ms) => {
			clock += ms;
		},
		pollMs: 1000,
		timeoutMs: 600_000,
		randomClaim: () => "c".repeat(32),
		assets: { dockerfile: "FROM scratch\n", entrypoint: "print(1)\n" },
		createWorkspace: async (owner, name) => {
			calls.push({ args: ["create", `${owner}/${name}`], stdin: "" });
			clock += 1000;
			const mode = opts.create ?? "ok";
			if (mode === "throw-none")
				throw new ProvisionError(
					"create_rejected",
					"workspace create rejected: HTTP 400: bad",
				);
			const w =
				mode === "prebuild"
					? mk({
							status: "starting",
							createdAt: new Date(T0 - 86_400_000).toISOString(),
						})
					: mode === "foreign"
						? mk({
								status: "starting",
								templateId: "99999999-0000-0000-0000-000000000000",
							})
						: mk({ status: "starting" });
			world.ws = w;
			if (mode === "throw-made")
				throw new Error("The operation was aborted due to timeout");
			return w;
		},
		getWorkspace: async (owner, name) => {
			assert.equal(`${owner}/${name}`, WS);
			// Advancing time makes the starting workspace come up on the next poll.
			if (world.ws?.status === "starting") {
				const w = world.ws;
				world.ws = { ...w, status: "running" };
				return w;
			}
			return world.ws;
		},
	};
	const log = (kind: string, data: Record<string, unknown>) => {
		logs.push({ kind, data });
	};
	const ops = () =>
		calls.map((c) =>
			c.args[0] === "ssh" ? `ssh:${JSON.parse(c.stdin).op}` : c.args[0],
		);
	return {
		dir,
		deps,
		log,
		calls,
		logs,
		world,
		ops,
		mk,
		cleanup: () => rmSync(dir, { recursive: true, force: true }),
	};
}

test("absent workspace: claim, create with fixed args, wait, check, prepare, return image", async () => {
	const h = harness({ checks: [false, true] });
	try {
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			IMAGE,
		);
		assert.deepEqual(h.ops(), [
			"create",
			"ssh:check",
			"ssh:check",
			"ssh:prepare",
		]);
		assert.deepEqual(h.calls[0].args, ["create", WS]);
		assert.deepEqual(h.calls[1].args, [
			"ssh",
			"--wait=no",
			"--disable-autostart",
			WS,
			"--",
			PREPARE_REMOTE_COMMAND,
		]);
		const prep = JSON.parse(h.calls[3].stdin);
		assert.deepEqual(prep, {
			op: "prepare",
			workspace: WS,
			session_id: SESSION,
			claim: "c".repeat(32),
			dockerfile: "FROM scratch\n",
			entrypoint: "print(1)\n",
		});
		const state = readProvisionState(h.dir)!;
		assert.equal(state.phase, "prepared");
		assert.equal(state.workspaceId, ID);
		assert.equal(state.image, IMAGE);
		assert.equal(state.absentCheckedAt, new Date(T0).toISOString());
		assert.ok(!JSON.stringify(h.logs).includes("FROM scratch"));
	} finally {
		h.cleanup();
	}
});

test("retry reuses the owned workspace without creating, and starts it when stopped", async () => {
	const h = harness();
	try {
		await prepareOnDemandWorkspace(
			{ workspace: WS, sessionId: SESSION },
			h.log,
			h.deps,
		);
		h.world.ws = { ...h.world.ws!, status: "stopped" };
		h.calls.length = 0;
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			IMAGE,
		);
		assert.deepEqual(h.ops(), ["start", "ssh:check", "ssh:prepare"]);
		assert.equal(JSON.parse(h.calls[2].stdin).claim, "c".repeat(32));
	} finally {
		h.cleanup();
	}
});

test("existing workspace without our claim is rejected and nothing is written", async () => {
	const h = harness({
		existing: { createdAt: new Date(T0 - 3_600_000).toISOString() },
	});
	try {
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			(e: ProvisionError) => e.code === "not_owned",
		);
		assert.deepEqual(h.calls, []);
		assert.equal(existsSync(join(h.dir, "provision.json")), false);
	} finally {
		h.cleanup();
	}
});

test("claimed workspace replaced by a different workspace ID is rejected", async () => {
	const h = harness();
	try {
		await prepareOnDemandWorkspace(
			{ workspace: WS, sessionId: SESSION },
			h.log,
			h.deps,
		);
		h.world.ws = { ...h.world.ws!, id: "99999999-2222-3333-4444-555555555555" };
		h.calls.length = 0;
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			(e: ProvisionError) => e.code === "not_owned",
		);
		assert.deepEqual(h.calls, []);
		h.world.ws = null;
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			(e: ProvisionError) => e.code === "workspace_missing",
		);
		assert.deepEqual(
			h.calls,
			[],
			"a deleted claimed workspace is not recreated",
		);
	} finally {
		h.cleanup();
	}
});

test("existing claim for another session or workspace is rejected", async () => {
	const h = harness();
	try {
		await prepareOnDemandWorkspace(
			{ workspace: WS, sessionId: SESSION },
			h.log,
			h.deps,
		);
		h.calls.length = 0;
		for (const input of [
			{ workspace: WS, sessionId: "sess_two" },
			{ workspace: "david-fraley/other", sessionId: SESSION },
		]) {
			await assert.rejects(
				prepareOnDemandWorkspace(input, h.log, h.deps),
				(e: ProvisionError) => e.code === "claim_conflict",
			);
		}
		assert.deepEqual(h.calls, []);
	} finally {
		h.cleanup();
	}
});

test("create failure is surfaced when no workspace appeared; claim survives for retry", async () => {
	const h = harness({ create: "throw-none" });
	try {
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			/workspace create failed: workspace create rejected: HTTP 400: bad/,
		);
		assert.equal(readProvisionState(h.dir)!.phase, "intent");
		assert.deepEqual(h.ops(), ["create"]);
	} finally {
		h.cleanup();
	}
});

test("create timeout reconciles to the workspace it actually created", async () => {
	const h = harness({ create: "throw-made" });
	try {
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			IMAGE,
		);
		assert.ok(h.logs.some((l) => l.kind === "provision_create_reconciled"));
		assert.equal(readProvisionState(h.dir)!.workspaceId, ID);
	} finally {
		h.cleanup();
	}
});

test("after an interrupted create, retry adopts only a workspace created after the absence check", async () => {
	const h = harness({ create: "throw-none" });
	try {
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
		);
		// Pre-dating the claim (or a different template) means it is not ours.
		for (const over of [
			{ createdAt: new Date(T0 - 3_600_000).toISOString() },
			{ templateId: "99999999-0000-0000-0000-000000000000" },
		]) {
			h.world.ws = h.mk(over);
			h.calls.length = 0;
			await assert.rejects(
				prepareOnDemandWorkspace(
					{ workspace: WS, sessionId: SESSION },
					h.log,
					h.deps,
				),
				(e: ProvisionError) => e.code === "not_owned",
			);
			assert.deepEqual(h.calls, []);
		}
		h.world.ws = h.mk();
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				h.log,
				h.deps,
			),
			IMAGE,
		);
	} finally {
		h.cleanup();
	}
});

test("invalid image ID from prepare is rejected and not recorded", async () => {
	for (const image of [
		"codex-webhook-executor:stage2",
		`sha256:${"A".repeat(64)}`,
		"sha256:abc",
		42,
	]) {
		const h = harness({ image });
		try {
			await assert.rejects(
				prepareOnDemandWorkspace(
					{ workspace: WS, sessionId: SESSION },
					h.log,
					h.deps,
				),
				(e: ProvisionError) => e.code === "invalid_image",
			);
			assert.equal(readProvisionState(h.dir)!.phase, "created");
		} finally {
			h.cleanup();
		}
	}
});

test("readiness polling is bounded by the overall deadline", async () => {
	const h = harness({ checks: [false] });
	try {
		await assert.rejects(
			prepareOnDemandWorkspace({ workspace: WS, sessionId: SESSION }, h.log, {
				...h.deps,
				timeoutMs: 30_000,
			}),
			/not ready: workspace is missing docker|timed out/,
		);
		assert.ok(!h.ops().includes("ssh:prepare"));
	} finally {
		h.cleanup();
	}
});

test("no template or workspace injection through inputs or environment", async () => {
	const h = harness();
	try {
		for (const workspace of [
			"--template=evil",
			"a/b --yes",
			"a/b/c",
			"a",
			"../x",
			"a/-b",
		]) {
			await assert.rejects(
				prepareOnDemandWorkspace(
					{ workspace, sessionId: SESSION },
					h.log,
					h.deps,
				),
				/owner\/name/,
			);
			assert.throws(() => buildPrepareSshArgs(workspace));
		}
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: "x; rm -rf /" },
				h.log,
				h.deps,
			),
			/invalid session id/,
		);
		assert.deepEqual(h.calls, []);
		const env = provisionEnv({
			CODER_URL: "u",
			CODER_SESSION_TOKEN: "t",
			CODER_TEMPLATE_VERSION: "v",
			CODER_RICH_PARAMETER: "x=y",
			CODER_PRESET_NAME: "p",
			CODER_AGENT_TOKEN: "a",
			OPENAI_API_KEY: "sk-x",
			PATH: "/bin",
			HOME: "/h",
		});
		assert.deepEqual(env, {
			CODER_URL: "u",
			CODER_SESSION_TOKEN: "t",
			PATH: "/bin",
			HOME: "/h",
		});
		assert.ok(
			!PREPARE_REMOTE_COMMAND.includes(WS) &&
				!PREPARE_REMOTE_COMMAND.includes(SESSION),
		);
	} finally {
		h.cleanup();
	}
});

test("fetchWorkspace: header-only token, 404 is absent, errors do not echo the token", async () => {
	const seen: { url: string; headers: Record<string, string> }[] = [];
	const body = {
		id: ID,
		owner_name: "david-fraley",
		name: "codex-worker",
		template_id: CODER_TEMPLATE_ID,
		template_name: "coder",
		created_at: "2026-10-05T12:00:00Z",
		latest_build: { status: "running" },
	};
	const fake = (status: number, json: unknown) =>
		(async (url: string, init: RequestInit) => {
			seen.push({ url, headers: init.headers as Record<string, string> });
			return new Response(JSON.stringify(json), { status });
		}) as unknown as typeof fetch;
	const env = {
		CODER_URL: "https://coder.example/",
		CODER_SESSION_TOKEN: "secret-token",
	};
	assert.deepEqual(
		await fetchWorkspace("david-fraley", "codex-worker", env, fake(200, body)),
		{
			id: ID,
			ownerName: "david-fraley",
			name: "codex-worker",
			templateId: CODER_TEMPLATE_ID,
			templateName: "coder",
			createdAt: "2026-10-05T12:00:00Z",
			status: "running",
		},
	);
	assert.equal(
		seen[0].url,
		"https://coder.example/api/v2/users/david-fraley/workspace/codex-worker",
	);
	assert.equal(seen[0].headers["Coder-Session-Token"], "secret-token");
	assert.equal(
		await fetchWorkspace("david-fraley", "codex-worker", env, fake(404, {})),
		null,
	);
	await assert.rejects(
		fetchWorkspace("david-fraley", "codex-worker", env, fake(500, {})),
		(e: Error) =>
			/HTTP 500/.test(e.message) && !e.message.includes("secret-token"),
	);
	await assert.rejects(
		fetchWorkspace(
			"david-fraley",
			"codex-worker",
			env,
			fake(200, { ...body, id: "nope" }),
		),
		/unexpected workspace shape/,
	);
	await assert.rejects(
		fetchWorkspace("david-fraley", "codex-worker", {}, fake(200, body)),
		/required/,
	);
});

test("create response is authoritative: a claimed prebuild is adopted by ID, a foreign template is rejected", async () => {
	const pre = harness({ create: "prebuild" });
	try {
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				pre.log,
				pre.deps,
			),
			IMAGE,
		);
		assert.equal(readProvisionState(pre.dir)!.workspaceId, ID);
		pre.calls.length = 0;
		assert.equal(
			await prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				pre.log,
				pre.deps,
			),
			IMAGE,
		);
		assert.ok(!pre.ops().includes("create"), "retry never creates again");
	} finally {
		pre.cleanup();
	}
	const foreign = harness({ create: "foreign" });
	try {
		await assert.rejects(
			prepareOnDemandWorkspace(
				{ workspace: WS, sessionId: SESSION },
				foreign.log,
				foreign.deps,
			),
			(e: ProvisionError) => e.code === "not_owned",
		);
		assert.equal(readProvisionState(foreign.dir)!.phase, "intent");
	} finally {
		foreign.cleanup();
	}
});

test("createWorkspaceViaApi: verifies owner, org, allowlisted template, default preset, then one POST", async () => {
	const ORG = "703f72a1-76f6-4f89-9de6-8a3989693fe5";
	const VER = "90b8f0fe-adc6-4454-b230-cc80842d3a53";
	const PRESET = "aab2df4e-516a-409c-bada-308f97a7023c";
	const created = {
		id: ID,
		owner_name: "david-fraley",
		name: "codex-worker",
		template_id: CODER_TEMPLATE_ID,
		template_name: "coder",
		created_at: "2026-10-05T12:00:00Z",
		latest_build: { status: "pending" },
	};
	const routes = (
		over: Record<string, [number, unknown]> = {},
	): Record<string, [number, unknown]> => ({
		"GET /api/v2/users/me": [
			200,
			{ username: "david-fraley", organization_ids: [ORG] },
		],
		"GET /api/v2/organizations/coder": [200, { id: ORG, name: "coder" }],
		[`GET /api/v2/organizations/${ORG}/templates/coder`]: [
			200,
			{
				id: CODER_TEMPLATE_ID,
				organization_id: ORG,
				deprecated: false,
				active_version_id: VER,
			},
		],
		[`GET /api/v2/templateversions/${VER}/presets`]: [
			200,
			[
				{ ID: "fb276626-8ede-480f-bd13-ebcf16c87019", Default: false },
				{ ID: PRESET, Default: true },
			],
		],
		[`POST /api/v2/organizations/${ORG}/members/david-fraley/workspaces`]: [
			201,
			created,
		],
		...over,
	});
	const env = {
		CODER_URL: "https://coder.example",
		CODER_SESSION_TOKEN: "secret-token",
	};
	const run = async (over: Record<string, [number, unknown]> = {}) => {
		const seen: { key: string; body?: unknown; token: string }[] = [];
		const table = routes(over);
		const fake = (async (url: string, init: RequestInit) => {
			const key = `${init.method} ${url.replace("https://coder.example", "")}`;
			seen.push({
				key,
				body: init.body ? JSON.parse(String(init.body)) : undefined,
				token: (init.headers as Record<string, string>)["Coder-Session-Token"],
			});
			const [status, json] = table[key] ?? [404, { message: "not found" }];
			return new Response(JSON.stringify(json), { status });
		}) as unknown as typeof fetch;
		let result: unknown;
		try {
			result = await createWorkspaceViaApi(
				"david-fraley",
				"codex-worker",
				env,
				fake,
			);
		} catch (err) {
			result = err;
		}
		return {
			result,
			seen,
			posts: seen.filter((s) => s.key.startsWith("POST")),
		};
	};

	const ok = await run();
	assert.equal((ok.result as CoderWorkspace).id, ID);
	assert.equal(ok.posts.length, 1);
	assert.deepEqual(ok.posts[0].body, {
		template_id: CODER_TEMPLATE_ID,
		name: "codex-worker",
		ttl_ms: 7_200_000,
		rich_parameter_values: [{ name: "Select IDEs", value: "[]" }],
		template_version_preset_id: PRESET,
	});
	assert.ok(ok.seen.every((s) => s.token === "secret-token"));
	assert.ok(!ok.seen.some((s) => s.key.includes("dry-run")));

	const checks: [Record<string, [number, unknown]>, string][] = [
		[
			{
				"GET /api/v2/users/me": [
					200,
					{ username: "someone-else", organization_ids: [ORG] },
				],
			},
			"not_owner",
		],
		[
			{
				"GET /api/v2/users/me": [
					200,
					{ username: "david-fraley", organization_ids: [] },
				],
			},
			"not_owner",
		],
		[
			{
				[`GET /api/v2/organizations/${ORG}/templates/coder`]: [
					200,
					{
						id: "99999999-0000-0000-0000-000000000000",
						organization_id: ORG,
						active_version_id: VER,
					},
				],
			},
			"template_mismatch",
		],
		[
			{
				[`GET /api/v2/organizations/${ORG}/templates/coder`]: [
					200,
					{
						id: CODER_TEMPLATE_ID,
						organization_id: ORG,
						deprecated: true,
						active_version_id: VER,
					},
				],
			},
			"template_mismatch",
		],
		[
			{
				[`GET /api/v2/templateversions/${VER}/presets`]: [
					200,
					[
						{ ID: PRESET, Default: true },
						{ ID: PRESET, Default: true },
					],
				],
			},
			"api_invalid",
		],
	];
	for (const [over, code] of checks) {
		const r = await run(over);
		assert.equal((r.result as ProvisionError).code, code, JSON.stringify(over));
		assert.equal(r.posts.length, 0, "no POST after a failed verification");
	}
	const noPreset = await run({
		[`GET /api/v2/templateversions/${VER}/presets`]: [200, []],
	});
	assert.equal(
		(noPreset.posts[0].body as Record<string, unknown>)
			.template_version_preset_id,
		undefined,
	);
	const conflict = await run({
		[`POST /api/v2/organizations/${ORG}/members/david-fraley/workspaces`]: [
			409,
			{ message: "exists" },
		],
	});
	assert.equal((conflict.result as ProvisionError).code, "create_conflict");
	const rejected = await run({
		[`POST /api/v2/organizations/${ORG}/members/david-fraley/workspaces`]: [
			400,
			{ message: "bad secret-token value" },
		],
	});
	assert.equal((rejected.result as ProvisionError).code, "create_rejected");
	assert.ok(!(rejected.result as Error).message.includes("secret-token"));
	for (const bad of ["a/b", "--x", "", "x y"])
		assert.throws(() => buildCreateBody(bad));
	assert.throws(() => buildCreateBody("ok", "not-a-uuid"));
});

const FAKE_DOCKER = String.raw`#!/usr/bin/env python3
import json, os, sys
# The helper strips unknown env vars, so locate fixtures relative to this script.
root = os.path.dirname(os.path.abspath(sys.argv[0]))
a = sys.argv[1:]
with open(root + '/docker.log', 'a') as f:
    f.write(json.dumps({'argv': a, 'env': sorted(os.environ)}) + '\n')
image = open(root + '/image').read().strip()
if a[0] == 'info':
    print('27.0')
elif a[0] == 'build':
    open(a[a.index('--iidfile') + 1], 'w').write(image)
elif a[:2] == ['image', 'inspect']:
    print(a[-1] + '|' + open(root + '/entrypoint').read().strip())
`;

const FAKE_NPM = String.raw`#!/usr/bin/env python3
import json, os, sys
root = os.path.dirname(os.path.dirname(os.path.abspath(sys.argv[0])))
a = sys.argv[1:]
with open(root + '/npm.log', 'a') as f:
    f.write(json.dumps({'argv': a, 'env': sorted(os.environ)}) + '\n')
if a and a[0] == 'install':
    prefix = a[a.index('--prefix') + 1]
    pkg = prefix + '/node_modules/@openai/codex'
    binary = prefix + '/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin'
    os.makedirs(pkg, exist_ok=True)
    os.makedirs(binary, exist_ok=True)
    json.dump({'version': a[-1].split('@')[-1]}, open(pkg + '/package.json', 'w'))
    open(binary + '/codex', 'w').write('#!/bin/sh\necho codex-cli\n')
    os.chmod(binary + '/codex', 0o755)
`;

test("prepare helper: receipt ownership, stable marker, pinned install, image ID, no secrets in subprocess env", () => {
	const root = mkdtempSync(join(tmpdir(), "prepare-helper-"));
	try {
		const home = join(root, "home");
		const bin = join(root, "bin");
		mkdirSync(home);
		mkdirSync(bin);
		writeFileSync(join(bin, "npm"), FAKE_NPM);
		chmodSync(join(bin, "npm"), 0o755);
		writeFileSync(join(root, "docker"), FAKE_DOCKER);
		chmodSync(join(root, "docker"), 0o755);
		writeFileSync(join(root, "image"), IMAGE);
		writeFileSync(
			join(root, "entrypoint"),
			'["python3","/usr/local/bin/executor-entrypoint.py"]',
		);
		const src = PREPARE_HELPER_SOURCE.replace(
			"HOME = '/home/coder'",
			`HOME = ${JSON.stringify(home)}`,
		).replace(
			"DOCKER = '/usr/bin/docker'",
			`DOCKER = ${JSON.stringify(join(root, "docker"))}`,
		);
		const run = (payload: Record<string, unknown>) => {
			const r = spawnSync("python3", ["-c", src], {
				input: `${JSON.stringify(payload)}\n`,
				timeout: 60_000,
				env: {
					PATH: `${bin}:${process.env.PATH}`,
					HOME: home,
					CODER_AGENT_TOKEN: "agent-secret",
					OPENAI_API_KEY: "sk-secret",
				},
			});
			const out = parseHelperOutput(r.stdout.toString());
			assert.ok(out, r.stderr.toString());
			return out;
		};
		const prep = {
			op: "prepare",
			workspace: WS,
			session_id: SESSION,
			claim: "c".repeat(32),
			dockerfile: "FROM x\n",
			entrypoint: "print(1)\n",
		};

		const check = run({ op: "check" });
		assert.equal(check.ok, true, JSON.stringify(check));
		assert.equal(run({ ...prep, claim: "not-hex" }).error, "invalid_input");
		const first = run(prep);
		assert.equal(first.ok, true, JSON.stringify(first));
		assert.equal(first.image, IMAGE);
		assert.equal(first.receipt_reused, false);
		const receipt = JSON.parse(
			readFileSync(join(home, ".codex-stage2-prepared.json"), "utf8"),
		);
		assert.match(receipt.marker, /^[0-9a-f]{32}$/);
		const marker = readFileSync(join(home, "demo/worker-marker.txt"), "utf8");
		assert.equal(marker, `codex-stage2 worker marker ${receipt.marker}\n`);
		assert.equal(
			readFileSync(
				join(home, ".codex-stage2-build/Dockerfile.executor"),
				"utf8",
			),
			"FROM x\n",
		);

		const second = run(prep);
		assert.equal(second.receipt_reused, true);
		assert.equal(second.codex_reused, true);
		assert.equal(
			readFileSync(join(home, "demo/worker-marker.txt"), "utf8"),
			marker,
			"marker identity survives retries",
		);
		assert.equal(run({ ...prep, session_id: "sess_two" }).error, "not_owner");
		assert.equal(run({ ...prep, claim: "d".repeat(32) }).error, "not_owner");

		const npmCalls = readFileSync(join(root, "npm.log"), "utf8")
			.trim()
			.split("\n")
			.map((l) => JSON.parse(l))
			.filter((c) => c.argv[0] === "install");
		assert.equal(npmCalls.length, 1);
		assert.deepEqual(npmCalls[0].argv, [
			"install",
			"--prefix",
			join(home, "codex-cli"),
			"--no-audit",
			"--no-fund",
			"--ignore-scripts",
			"--save-exact",
			"@openai/codex@0.156.0-alpha.2",
		]);
		const dockerCalls = readFileSync(join(root, "docker.log"), "utf8")
			.trim()
			.split("\n")
			.map((l) => JSON.parse(l));
		const build = dockerCalls.find((c) => c.argv[0] === "build");
		assert.deepEqual(build.argv.slice(0, 5), [
			"build",
			"-f",
			join(home, ".codex-stage2-build/Dockerfile.executor"),
			"-t",
			"codex-webhook-executor:stage2",
		]);
		for (const c of [...npmCalls, ...dockerCalls]) {
			assert.ok(
				!c.env.includes("CODER_AGENT_TOKEN") &&
					!c.env.includes("OPENAI_API_KEY"),
				JSON.stringify(c.env),
			);
		}

		writeFileSync(join(root, "entrypoint"), '["/bin/sh"]');
		assert.equal(run(prep).error, "build_failed");
		writeFileSync(join(root, "image"), "not-an-id");
		assert.equal(run(prep).error, "build_failed");

		// A workspace whose demo directory predates any receipt is not ours.
		rmSync(join(home, ".codex-stage2-prepared.json"));
		assert.equal(run(prep).error, "not_owner");
	} finally {
		rmSync(root, { recursive: true, force: true });
	}
});

import {
	clampContent,
	coerceRunSummary,
	coerceStepRequest,
	coerceStepResponse,
	coerceUsageRecord,
	compactDuration,
	computeDurationMs,
	extractTokenCounts,
	formatTokenSummary,
	getRoleBadgeVariant,
	getRunKindLabel,
	getStatusBadgeVariant,
	isActiveStatus,
	normalizeAttempts,
} from "./debugPanelUtils";

describe("coerceStepResponse", () => {
	it("keeps tool-result content emitted in normalized response parts", () => {
		const response = coerceStepResponse({
			content: [
				{
					type: "tool-result",
					tool_call_id: "call-1",
					tool_name: "search_docs",
					result: {
						matches: ["model.go", "debugPanelUtils.ts"],
					},
				},
			],
		});

		const parsed = JSON.parse(response.content);
		expect(parsed).toEqual({
			matches: ["model.go", "debugPanelUtils.ts"],
		});
		expect(response.toolCalls).toEqual([]);
		expect(response.usage).toEqual({});
	});

	it.each([
		["numeric zero", 0, "0"],
		["boolean false", false, "false"],
		["explicit null", null, "null"],
	])("preserves primitive tool-result %s in debug payloads", (_label, result, expected) => {
		const response = coerceStepResponse({
			content: [
				{
					type: "tool-result",
					tool_call_id: "call-1",
					tool_name: "probe",
					result,
				},
			],
		});

		expect(response.content).toBe(expected);
	});

	it("extracts tool_input streaming deltas as tool calls", () => {
		// Interrupted streams emit `tool_input` parts with the accumulated
		// arguments before a final `tool_call` summary exists.
		const response = coerceStepResponse({
			content: [
				{
					type: "tool_input",
					tool_call_id: "call-42",
					tool_name: "search_docs",
					arguments: '{"query":"foo"}',
				},
			],
		});

		expect(response.toolCalls).toEqual([
			{
				id: "call-42",
				name: "search_docs",
				arguments: '{\n  "query": "foo"\n}',
			},
		]);
	});

	it("prefers finalized tool_call over the streaming tool_input delta for the same call ID", () => {
		const response = coerceStepResponse({
			content: [
				{
					type: "tool_input",
					tool_call_id: "call-42",
					tool_name: "search_docs",
					arguments: '{"query":"f',
				},
				{
					type: "tool_call",
					tool_call_id: "call-42",
					tool_name: "search_docs",
					arguments: '{"query":"foo"}',
				},
			],
		});

		expect(response.toolCalls).toEqual([
			{
				id: "call-42",
				name: "search_docs",
				arguments: '{\n  "query": "foo"\n}',
			},
		]);
	});

	it("keeps the finalized payload when tool_call precedes a stray tool_input for the same ID", () => {
		const response = coerceStepResponse({
			content: [
				{
					type: "tool_call",
					tool_call_id: "call-42",
					tool_name: "search_docs",
					arguments: '{"query":"foo"}',
				},
				{
					type: "tool_input",
					tool_call_id: "call-42",
					tool_name: "search_docs",
					arguments: '{"query":"bar"}',
				},
			],
		});

		expect(response.toolCalls).toEqual([
			{
				id: "call-42",
				name: "search_docs",
				arguments: '{\n  "query": "foo"\n}',
			},
		]);
	});

	it("keeps per-call entries when multiple distinct tool calls are emitted", () => {
		const response = coerceStepResponse({
			content: [
				{
					type: "tool_input",
					tool_call_id: "call-1",
					tool_name: "search_docs",
					arguments: '{"query":"a"}',
				},
				{
					type: "tool_input",
					tool_call_id: "call-2",
					tool_name: "calc",
					arguments: '{"op":"add"}',
				},
			],
		});

		expect(response.toolCalls).toEqual([
			{
				id: "call-1",
				name: "search_docs",
				arguments: '{\n  "query": "a"\n}',
			},
			{
				id: "call-2",
				name: "calc",
				arguments: '{\n  "op": "add"\n}',
			},
		]);
	});
});

describe("getRunKindLabel", () => {
	it.each([
		["chat_turn", "Chat Turn"],
		["title_generation", "Title Generation"],
		["compaction", "Compaction"],
		["quickgen", "Quick Gen"],
		["quick_gen", "Quick Gen"],
		["llm_call", "LLM Call"],
		["post_process", "Post-process"],
		["tool_call", "Tool Call"],
	])("maps %s to the canonical label", (kind, label) => {
		expect(getRunKindLabel(kind)).toBe(label);
	});

	it("humanizes unknown kinds with title casing", () => {
		expect(getRunKindLabel("custom_kind")).toBe("Custom Kind");
	});

	it("returns Unknown for blank input", () => {
		expect(getRunKindLabel("   ")).toBe("Unknown");
	});
});

describe("getStatusBadgeVariant", () => {
	it.each([
		["completed", "green"],
		["SUCCESS", "green"],
		["failed", "destructive"],
		["interrupted", "destructive"],
		["cancelled", "destructive"],
		["canceled", "destructive"],
		["running", "info"],
		["in_progress", "info"],
		["pending", "warning"],
		["queued", "warning"],
		["mystery", "default"],
	])("maps %s to %s", (status, expected) => {
		expect(getStatusBadgeVariant(status)).toBe(expected);
	});
});

describe("isActiveStatus", () => {
	it.each([
		["running", true],
		["in_progress", true],
		["processing", true],
		["started", true],
		["completed", false],
		["pending", false],
	])("returns %s-active=%s", (status, expected) => {
		expect(isActiveStatus(status)).toBe(expected);
	});
});

describe("getRoleBadgeVariant", () => {
	it.each([
		["system", "purple"],
		["user", "info"],
		["assistant", "green"],
		["tool", "warning"],
		["function", "warning"],
		["unknown", "default"],
	])("maps %s to %s", (role, expected) => {
		expect(getRoleBadgeVariant(role)).toBe(expected);
	});
});

describe("normalizeAttempts", () => {
	it("parses array input and sorts by attempt_number", () => {
		const result = normalizeAttempts([
			{ number: 2, status: "completed" },
			{ attempt_number: 1, status: "error" },
		]);

		expect(result.rawFallback).toBeUndefined();
		expect(result.parsed.map((a) => a.attempt_number)).toEqual([1, 2]);
		expect(result.parsed.map((a) => a.status)).toEqual(["error", "completed"]);
	});

	it("parses JSON strings that wrap an array of attempts", () => {
		const result = normalizeAttempts(
			JSON.stringify([
				{ attempt_number: 1, status: "completed", method: "POST" },
			]),
		);

		expect(result.rawFallback).toBeUndefined();
		expect(result.parsed).toEqual([
			expect.objectContaining({
				attempt_number: 1,
				status: "completed",
				method: "POST",
			}),
		]);
	});

	it("returns an empty array for empty input without a raw fallback", () => {
		expect(normalizeAttempts([])).toEqual({ parsed: [] });
		expect(normalizeAttempts({})).toEqual({ parsed: [] });
	});

	it("parses record-shaped attempts keyed by index", () => {
		const result = normalizeAttempts({
			"1": { attempt_number: 1, status: "completed" },
			"2": { attempt_number: 2, status: "error" },
		});

		expect(result.rawFallback).toBeUndefined();
		expect(result.parsed.map((a) => a.attempt_number)).toEqual([1, 2]);
	});

	it("returns raw fallback for unparsable strings", () => {
		const result = normalizeAttempts("not json");
		expect(result.parsed).toEqual([]);
		expect(result.rawFallback).toBe("not json");
	});

	it("returns raw fallback for unsupported types", () => {
		const result = normalizeAttempts(42);
		expect(result.parsed).toEqual([]);
		expect(result.rawFallback).toBe("42");
	});

	it("decodes base64-encoded request bodies into JSON", () => {
		// {"prompt":"hi"} encoded as base64.
		const encodedBody = btoa('{"prompt":"hi"}');
		const [attempt] = normalizeAttempts([
			{
				attempt_number: 1,
				status: "completed",
				request_body: encodedBody,
			},
		]).parsed;

		expect(attempt?.raw_request).toEqual({ body: { prompt: "hi" } });
	});

	it("preserves plain-text bodies that happen to be base64-alphabet", () => {
		// "test" is in the base64 alphabet and has length 4, but it is
		// almost certainly a literal payload. Decoding it would produce
		// mojibake (0xB5 0xEB 0x2D is not valid UTF-8).
		const [attempt] = normalizeAttempts([
			{
				attempt_number: 1,
				status: "completed",
				request_body: "test",
				response_body: "abcd",
			},
		]).parsed;

		expect(attempt?.raw_request).toEqual({ body: "test" });
		expect(attempt?.raw_response).toEqual({ body: "abcd" });
	});

	it("decodes base64-encoded non-JSON text", () => {
		// Go can emit non-JSON []byte payloads (e.g. plain-text error
		// bodies). Once step 2 fails JSON parsing, step 3 should return
		// the decoded UTF-8 text.
		const encodedBody = btoa("hello world");
		const [attempt] = normalizeAttempts([
			{
				attempt_number: 1,
				status: "completed",
				response_body: encodedBody,
			},
		]).parsed;

		expect(attempt?.raw_response).toEqual({ body: "hello world" });
	});
});

describe("computeDurationMs", () => {
	it("computes elapsed time between two ISO timestamps", () => {
		expect(
			computeDurationMs("2024-01-01T00:00:00.000Z", "2024-01-01T00:00:02.500Z"),
		).toBe(2500);
	});

	it("returns null when startedAt is not parseable", () => {
		expect(computeDurationMs("not-a-date")).toBeNull();
	});

	it("returns null when finishedAt is provided but not parseable", () => {
		expect(
			computeDurationMs("2024-01-01T00:00:00.000Z", "also-not-a-date"),
		).toBeNull();
	});

	it("clamps negative durations to zero", () => {
		expect(
			computeDurationMs("2024-01-01T00:00:10.000Z", "2024-01-01T00:00:05.000Z"),
		).toBe(0);
	});

	it("falls back to current time when finishedAt is omitted", () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2024-01-01T00:00:05.000Z"));
		try {
			expect(computeDurationMs("2024-01-01T00:00:00.000Z")).toBe(5000);
		} finally {
			vi.useRealTimers();
		}
	});
});

describe("compactDuration", () => {
	it.each([
		[0, "0ms"],
		[999, "999ms"],
		[1000, "1.0s"],
		[1500, "1.5s"],
		[59999, "60.0s"],
		[60000, "1m"],
		[61000, "1m 1s"],
		[125000, "2m 5s"],
	])("formats %sms as %s", (ms, expected) => {
		expect(compactDuration(ms)).toBe(expected);
	});
});

describe("formatTokenSummary", () => {
	it("renders both input and output counts", () => {
		expect(formatTokenSummary(1200, 340)).toBe("1,200→340 tok");
	});

	it("renders input-only when output is undefined", () => {
		expect(formatTokenSummary(1200, undefined)).toBe("1,200 in");
	});

	it("renders output-only when input is undefined", () => {
		expect(formatTokenSummary(undefined, 340)).toBe("340 out");
	});

	it("returns an empty string when both counts are undefined", () => {
		expect(formatTokenSummary()).toBe("");
	});
});

describe("extractTokenCounts", () => {
	it("prefers prompt/completion keys when present", () => {
		expect(
			extractTokenCounts({
				prompt_tokens: 10,
				completion_tokens: 20,
				total_tokens: 30,
				input_tokens: 99,
				output_tokens: 99,
			}),
		).toEqual({ input: 10, output: 20, total: 30 });
	});

	it("falls back to input/output_tokens when prompt/completion are absent", () => {
		expect(
			extractTokenCounts({
				input_tokens: 5,
				output_tokens: 7,
			}),
		).toEqual({ input: 5, output: 7, total: undefined });
	});
});

describe("coerceUsageRecord", () => {
	it("coerces string numeric values to numbers", () => {
		expect(
			coerceUsageRecord({ prompt_tokens: "10", completion_tokens: 20 }),
		).toEqual({ prompt_tokens: 10, completion_tokens: 20 });
	});

	it("drops non-finite values", () => {
		expect(coerceUsageRecord({ a: "abc", b: null, c: 5 })).toEqual({ c: 5 });
	});

	it("parses usage embedded as a JSON string", () => {
		expect(coerceUsageRecord('{"prompt_tokens": 3}')).toEqual({
			prompt_tokens: 3,
		});
	});

	it("returns an empty record for non-object input", () => {
		expect(coerceUsageRecord(null)).toEqual({});
		expect(coerceUsageRecord(42)).toEqual({});
	});
});

describe("coerceRunSummary", () => {
	it("extracts the primary label and token counts from snake_case fields", () => {
		const summary = coerceRunSummary({
			first_message: "Hello",
			endpoint_label: "openai/chat",
			model: "gpt-4",
			provider: "openai",
			step_count: 3,
			total_input_tokens: 120,
			total_output_tokens: 45,
		});

		expect(summary).toEqual({
			primaryLabel: "Hello",
			endpointLabel: "openai/chat",
			model: "gpt-4",
			provider: "openai",
			stepCount: 3,
			totalInputTokens: 120,
			totalOutputTokens: 45,
			warnings: [],
		});
	});

	it("falls back to camelCase and alternate token names", () => {
		const summary = coerceRunSummary({
			primaryLabel: "Fallback",
			promptTokens: "90",
			completionTokens: "30",
		});

		expect(summary.primaryLabel).toBe("Fallback");
		expect(summary.totalInputTokens).toBe(90);
		expect(summary.totalOutputTokens).toBe(30);
	});

	it("returns defaults for non-object input", () => {
		expect(coerceRunSummary(null)).toEqual({
			primaryLabel: "",
			endpointLabel: undefined,
			model: undefined,
			provider: undefined,
			stepCount: undefined,
			totalInputTokens: undefined,
			totalOutputTokens: undefined,
			warnings: [],
		});
	});
});

describe("coerceStepRequest", () => {
	it("coerces messages, tools, and options nested under options/policy", () => {
		const request = coerceStepRequest({
			model: "gpt-4",
			messages: [
				{ role: "system", content: "Be helpful" },
				{ role: "user", parts: [{ type: "text", text: "Hi" }] },
			],
			tools: [
				{
					type: "function",
					function: {
						name: "search_docs",
						description: "Search the docs",
						parameters: { type: "object" },
					},
				},
			],
			options: {
				temperature: 0.2,
				max_output_tokens: 512,
				ignored_field: "drop me",
			},
			policy: {
				tool_choice: "auto",
				parallel_tool_calls: true,
			},
		});

		expect(request.model).toBe("gpt-4");
		expect(request.messages).toHaveLength(2);
		expect(request.messages[0]).toMatchObject({
			role: "system",
			content: "Be helpful",
		});
		expect(request.messages[1]).toMatchObject({ role: "user", content: "Hi" });
		expect(request.tools).toEqual([
			{
				name: "search_docs",
				description: "Search the docs",
				inputSchema: expect.any(String),
			},
		]);
		expect(request.options).toEqual({
			temperature: 0.2,
			max_output_tokens: 512,
		});
		expect(request.policy).toEqual({
			tool_choice: "auto",
			parallel_tool_calls: true,
		});
	});

	it("falls back to top-level option fields when no options wrapper is present", () => {
		const request = coerceStepRequest({
			temperature: 0.7,
			top_p: 0.9,
		});

		expect(request.options).toEqual({ temperature: 0.7, top_p: 0.9 });
	});

	it("returns defaults for non-object input", () => {
		expect(coerceStepRequest(null)).toEqual({
			model: undefined,
			messages: [],
			tools: [],
			options: {},
			policy: {},
		});
	});
});

describe("clampContent", () => {
	it("returns the trimmed text when under the limit", () => {
		expect(clampContent("  hello  ", 20)).toBe("hello");
	});

	it("truncates and appends an ellipsis when over the limit", () => {
		expect(clampContent("hello world", 5)).toBe("hello…");
	});
});

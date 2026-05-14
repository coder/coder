import type { UserSecret } from "#/api/typesGenerated";
import {
	importUserSecretsSequential,
	parseSecretImport,
} from "./importSecrets";

describe("parseSecretImport", () => {
	it("parses .env files", () => {
		expect(
			parseSecretImport(
				[
					"# comment",
					"GITHUB_TOKEN=example-value",
					"",
					"ANTHROPIC_API_KEY=another-example-value",
				].join("\n"),
				"secrets.env",
			),
		).toEqual([
			{
				name: "GITHUB_TOKEN",
				env_name: "GITHUB_TOKEN",
				value: "example-value",
			},
			{
				name: "ANTHROPIC_API_KEY",
				env_name: "ANTHROPIC_API_KEY",
				value: "another-example-value",
			},
		]);
	});

	it("preserves unquoted .env value whitespace", () => {
		expect(
			parseSecretImport(
				["SPACEY=  leading and trailing  ", "ONLY_SPACES=   "].join("\n"),
				"secrets.env",
			),
		).toEqual([
			{
				name: "SPACEY",
				env_name: "SPACEY",
				value: "  leading and trailing  ",
			},
			{
				name: "ONLY_SPACES",
				env_name: "ONLY_SPACES",
				value: "   ",
			},
		]);
	});

	it("parses quoted .env values", () => {
		expect(
			parseSecretImport(
				[
					'DOUBLE_QUOTED="example=value"',
					"SINGLE_QUOTED='example value'",
					'DOUBLE_QUOTED_SPACES="  keep me  "',
					'DOUBLE_QUOTED_ESCAPE="line\\nnext"',
				].join("\n"),
				"secrets.env",
			),
		).toEqual([
			{
				name: "DOUBLE_QUOTED",
				env_name: "DOUBLE_QUOTED",
				value: "example=value",
			},
			{
				name: "SINGLE_QUOTED",
				env_name: "SINGLE_QUOTED",
				value: "example value",
			},
			{
				name: "DOUBLE_QUOTED_SPACES",
				env_name: "DOUBLE_QUOTED_SPACES",
				value: "  keep me  ",
			},
			{
				name: "DOUBLE_QUOTED_ESCAPE",
				env_name: "DOUBLE_QUOTED_ESCAPE",
				value: "line\nnext",
			},
		]);
	});

	it("parses JSON object maps", () => {
		expect(
			parseSecretImport(
				JSON.stringify({
					GITHUB_TOKEN: "example-value",
					ANTHROPIC_API_KEY: "another-example-value",
				}),
				"secrets.json",
			),
		).toEqual([
			{
				name: "GITHUB_TOKEN",
				env_name: "GITHUB_TOKEN",
				value: "example-value",
			},
			{
				name: "ANTHROPIC_API_KEY",
				env_name: "ANTHROPIC_API_KEY",
				value: "another-example-value",
			},
		]);
	});

	it("parses JSON arrays of secret requests", () => {
		expect(
			parseSecretImport(
				JSON.stringify([
					{
						name: "github",
						value: "example-value",
						env_name: "GITHUB_TOKEN",
						description: "GitHub token",
					},
				]),
				"secrets.json",
			),
		).toEqual([
			{
				name: "github",
				value: "example-value",
				env_name: "GITHUB_TOKEN",
				description: "GitHub token",
			},
		]);
	});

	it("parses YAML object maps", () => {
		expect(
			parseSecretImport(
				[
					"GITHUB_TOKEN: example-value",
					"ANTHROPIC_API_KEY: another-example-value",
				].join("\n"),
				"secrets.yaml",
			),
		).toEqual([
			{
				name: "GITHUB_TOKEN",
				env_name: "GITHUB_TOKEN",
				value: "example-value",
			},
			{
				name: "ANTHROPIC_API_KEY",
				env_name: "ANTHROPIC_API_KEY",
				value: "another-example-value",
			},
		]);
	});

	it("parses YAML arrays of secret requests", () => {
		expect(
			parseSecretImport(
				[
					"- name: github",
					"  value: example-value",
					"  env_name: GITHUB_TOKEN",
					"  description: GitHub token",
					"  file_path: ~/secrets/github",
				].join("\n"),
				"secrets.yml",
			),
		).toEqual([
			{
				name: "github",
				value: "example-value",
				env_name: "GITHUB_TOKEN",
				description: "GitHub token",
				file_path: "~/secrets/github",
			},
		]);
	});

	it.each([
		["ROUTE/UNSAFE=placeholder", "secrets.env"],
		[JSON.stringify({ "ROUTE?UNSAFE": "placeholder" }), "secrets.json"],
		[
			JSON.stringify([{ name: "ROUTE#UNSAFE", value: "placeholder" }]),
			"secrets.json",
		],
		["ROUTE/UNSAFE: placeholder", "secrets.yaml"],
		[
			["- name: ROUTE#UNSAFE", "  value: placeholder"].join("\n"),
			"secrets.yaml",
		],
	])("rejects route-unsafe imported secret names", (content, fileName) => {
		expect(() => parseSecretImport(content, fileName)).toThrow("Invalid");
	});

	it.each([
		["1PASSWORD=value", "secrets.env"],
		[JSON.stringify({ "my-secret": "value" }), "secrets.json"],
		["my-secret: value", "secrets.yaml"],
		[
			JSON.stringify([{ name: "ok", value: "value", env_name: "1BAD" }]),
			"secrets.json",
		],
		[
			["- name: ok", "  value: value", "  file_path: relative/path"].join("\n"),
			"secrets.yaml",
		],
	])("rejects invalid imported targets", (content, fileName) => {
		expect(() => parseSecretImport(content, fileName)).toThrow(
			"Invalid secret import.",
		);
	});

	it.each([
		[
			"GITHUB_TOKEN: true",
			"YAML secret value for GITHUB_TOKEN must be a string, got boolean.",
		],
		[
			"GITHUB_TOKEN: 123",
			"YAML secret value for GITHUB_TOKEN must be a string, got number.",
		],
		[
			["- name: github", "  value: true"].join("\n"),
			"YAML secret value for github at index 0 must be a string, got boolean.",
		],
	])("rejects non-string YAML secret values", (content, error) => {
		expect(() => parseSecretImport(content, "secrets.yaml")).toThrow(error);
	});

	it("rejects non-string JSON secret values", () => {
		expect(() =>
			parseSecretImport(
				JSON.stringify({ GITHUB_TOKEN: { nested: "value" } }),
				"secrets.json",
			),
		).toThrow(
			"JSON secret value for GITHUB_TOKEN must be a string, got object.",
		);
	});
});

describe("importUserSecretsSequential", () => {
	it("keeps successful results and reports failed secrets", async () => {
		const createdSecret: UserSecret = {
			id: "11111111-1111-1111-1111-111111111111",
			name: "GITHUB_TOKEN",
			description: "",
			env_name: "GITHUB_TOKEN",
			file_path: "",
			created_at: "2026-05-04T00:00:00Z",
			updated_at: "2026-05-04T00:00:00Z",
		};
		const createSecret = vi
			.fn()
			.mockResolvedValueOnce(createdSecret)
			.mockRejectedValueOnce(new Error("Create failed."));

		const results = await importUserSecretsSequential(
			[
				{
					name: "GITHUB_TOKEN",
					env_name: "GITHUB_TOKEN",
					value: "example-value",
				},
				{
					name: "ANTHROPIC_API_KEY",
					env_name: "ANTHROPIC_API_KEY",
					value: "another-example-value",
				},
			],
			createSecret,
		);

		expect(results).toMatchObject([
			{
				status: "success",
				name: "GITHUB_TOKEN",
				secret: createdSecret,
			},
			{
				status: "failure",
				name: "ANTHROPIC_API_KEY",
			},
		]);
		expect(createSecret).toHaveBeenCalledTimes(2);
	});
});

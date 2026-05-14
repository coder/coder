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
		["GITHUB_TOKEN: true"],
		["GITHUB_TOKEN: 123"],
		[["- name: github", "  value: true"].join("\n")],
	])("rejects non-string YAML secret values", (content) => {
		expect(() => parseSecretImport(content, "secrets.yaml")).toThrow(
			"Invalid YAML secret value",
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

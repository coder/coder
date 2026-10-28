import type { FoundationOutputMetadata } from "#/api/typesGenerated";
import { MockFoundationWithOutputs } from "#/testHelpers/templateResources";
import { getFoundationOutputs } from "./foundationOutputs";

describe("getFoundationOutputs", () => {
	it("includes metadata-only secrets and preserves all public value types", () => {
		expect(getFoundationOutputs(MockFoundationWithOutputs)).toEqual([
			{
				name: "credentials",
				type: '["map","string"]',
				sensitivity: "Sensitive",
				value: "********",
			},
			{
				name: "enabled",
				type: '"bool"',
				sensitivity: "Public",
				value: "false",
			},
			{
				name: "endpoint",
				type: '"string"',
				sensitivity: "Public",
				value: '"https://example.com"',
			},
			{
				name: "optional",
				type: '"string"',
				sensitivity: "Public",
				value: "null",
			},
			{
				name: "password",
				type: '"string"',
				sensitivity: "Sensitive",
				value: "********",
			},
			{
				name: "replicas",
				type: '"number"',
				sensitivity: "Public",
				value: "0",
			},
			{
				name: "settings",
				type: '["object",{"retries":"number","labels":["map","string"]}]',
				sensitivity: "Public",
				value:
					'{\n  "retries": 3,\n  "labels": {\n    "env": "development"\n  }\n}',
			},
			{
				name: "zones",
				type: '["list","string"]',
				sensitivity: "Public",
				value: '[\n  "us-east-1a",\n  "us-east-1b"\n]',
			},
		]);
	});

	it.each(["must-not-be-disclosed", { token: "must-not-be-disclosed" }, null])(
		"masks sensitive values accidentally present in the public map: %j",
		(value) => {
			expect(
				getFoundationOutputs({
					outputs: { password: value },
					output_metadata: { password: { type: "string", sensitive: true } },
				}),
			).toEqual([
				{
					name: "password",
					type: '"string"',
					sensitivity: "Sensitive",
					value: "********",
				},
			]);
		},
	);

	it.each<Record<string, FoundationOutputMetadata>>([
		{},
		{ password: { sensitive: true } },
	])("never reads values without explicit public metadata: %j", (metadata) => {
		const readSecret = vi.fn(() => "must-not-be-disclosed");
		const outputs = {
			get password() {
				return readSecret();
			},
		};
		expect(
			getFoundationOutputs({ outputs, output_metadata: metadata }),
		).toEqual([
			{
				name: "password",
				type: "Unknown",
				sensitivity: metadata.password ? "Sensitive" : "Unknown",
				value: "********",
			},
		]);
		expect(readSecret).not.toHaveBeenCalled();
	});

	it("distinguishes a missing public value from null", () => {
		expect(
			getFoundationOutputs({
				outputs: {},
				output_metadata: { endpoint: { type: "string", sensitive: false } },
			}),
		).toEqual([
			{
				name: "endpoint",
				type: '"string"',
				sensitivity: "Public",
				value: "Unavailable",
			},
		]);
	});

	it("returns no rows for an empty snapshot", () => {
		expect(getFoundationOutputs({ outputs: {}, output_metadata: {} })).toEqual(
			[],
		);
	});
});

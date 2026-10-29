import { describe, expect, it } from "vitest";
import { MockCoderMCPServer } from "../testFixtures";
import {
	buildCreateMCPServerConfigRequest,
	buildInitialMCPServerFormValues,
	buildUpdateMCPServerConfigRequest,
	canSubmitMCPServerForm,
	type MCPServerFormValues,
	SECRET_PLACEHOLDER,
} from "./mcpServerFormLogic";

const validValues = (
	overrides: Partial<MCPServerFormValues> = {},
): MCPServerFormValues => ({
	...buildInitialMCPServerFormValues(),
	displayName: "GitHub",
	slug: "github",
	url: "https://api.githubcopilot.com/mcp/",
	...overrides,
});

describe("mcpServerFormLogic", () => {
	it("uses placeholders for existing secrets", () => {
		const values = buildInitialMCPServerFormValues({
			...MockCoderMCPServer,
			has_api_key: true,
		});

		expect(values.oauth2ClientSecret).toBe(SECRET_PLACEHOLDER);
		expect(values.apiKeyValue).toBe(SECRET_PLACEHOLDER);
		expect(values.signingSecret).toBe(SECRET_PLACEHOLDER);
	});

	it("requires display name, slug, and URL before submitting", () => {
		expect(canSubmitMCPServerForm(validValues(), false)).toBe(true);
		expect(
			canSubmitMCPServerForm(validValues({ displayName: "" }), false),
		).toBe(false);
		expect(canSubmitMCPServerForm(validValues({ slug: "" }), false)).toBe(
			false,
		);
		expect(canSubmitMCPServerForm(validValues({ url: "" }), false)).toBe(false);
		expect(canSubmitMCPServerForm(validValues(), true)).toBe(false);
	});

	it("does not send placeholder OAuth2 secrets unless the value changes", () => {
		const unchanged = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "oauth2",
				oauth2ClientSecret: SECRET_PLACEHOLDER,
				oauth2SecretTouched: false,
			}),
		);
		const changed = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "oauth2",
				oauth2ClientSecret: "new-secret",
				oauth2SecretTouched: true,
			}),
		);

		expect(unchanged.oauth2_client_secret).toBeUndefined();
		expect(changed.oauth2_client_secret).toBe("new-secret");
	});

	it("does not send placeholder API key values unless the value changes", () => {
		const unchanged = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "api_key",
				apiKeyValue: SECRET_PLACEHOLDER,
				apiKeyTouched: false,
			}),
		);
		const changed = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "api_key",
				apiKeyValue: "new-key",
				apiKeyTouched: true,
			}),
		);

		expect(unchanged.api_key_value).toBeUndefined();
		expect(changed.api_key_value).toBe("new-key");
	});

	it.each([
		{ value: SECRET_PLACEHOLDER, touched: false, expected: undefined },
		{ value: SECRET_PLACEHOLDER, touched: true, expected: undefined },
		{ value: "", touched: true, expected: undefined },
		{
			value: "new-signing-secret",
			touched: true,
			expected: "new-signing-secret",
		},
	])(
		"sends only a replacement signing secret: $value/$touched",
		({ value, touched, expected }) => {
			const values = validValues({
				signingSecret: value,
				signingSecretTouched: touched,
			});
			expect(buildCreateMCPServerConfigRequest(values).signing_secret).toBe(
				expected,
			);
			expect(buildUpdateMCPServerConfigRequest(values).signing_secret).toBe(
				expected,
			);
		},
	);

	it("omits enabled from update requests", () => {
		const request = buildUpdateMCPServerConfigRequest(
			validValues({ enabled: false }),
		);
		expect(request.enabled).toBeUndefined();
	});

	it("sends empty revocation URL and resource on update so they can be cleared", () => {
		const clearedOAuth2Values = validValues({
			authType: "oauth2",
			oauth2RevocationURL: "",
			oauth2Resource: " ",
		});
		const cleared = buildUpdateMCPServerConfigRequest(clearedOAuth2Values);
		expect(cleared.oauth2_revocation_url).toBe("");
		expect(cleared.oauth2_resource).toBe("");

		const created = buildCreateMCPServerConfigRequest(clearedOAuth2Values);
		expect(created.oauth2_revocation_url).toBeUndefined();
		expect(created.oauth2_resource).toBeUndefined();
	});

	it("round-trips the stored OAuth2 resource", () => {
		const values = buildInitialMCPServerFormValues({
			...MockCoderMCPServer,
			oauth2_resource: "https://mcp.example.com/mcp",
		});
		expect(values.oauth2Resource).toBe("https://mcp.example.com/mcp");
		expect(buildUpdateMCPServerConfigRequest(values).oauth2_resource).toBe(
			"https://mcp.example.com/mcp",
		);
		expect(buildCreateMCPServerConfigRequest(values).oauth2_resource).toBe(
			"https://mcp.example.com/mcp",
		);
	});

	it("initializes slugTouched true for edit and false for create", () => {
		const createValues = buildInitialMCPServerFormValues();
		expect(createValues.slugTouched).toBe(false);
		const editValues = buildInitialMCPServerFormValues(MockCoderMCPServer);
		expect(editValues.slugTouched).toBe(true);
	});

	it("only sends touched custom headers with non-empty keys", () => {
		const untouched = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "custom_headers",
				customHeadersTouched: false,
				customHeaders: [{ key: "X-Test", value: "secret" }],
			}),
		);
		const touched = buildCreateMCPServerConfigRequest(
			validValues({
				authType: "custom_headers",
				customHeadersTouched: true,
				customHeaders: [
					{ key: "X-Test", value: "secret" },
					{ key: " ", value: "ignored" },
				],
			}),
		);

		expect(untouched.custom_headers).toBeUndefined();
		expect(touched.custom_headers).toEqual({ "X-Test": "secret" });
	});
});

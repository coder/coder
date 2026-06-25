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

	it("omits enabled from update requests", () => {
		const request = buildUpdateMCPServerConfigRequest(
			validValues({ enabled: false }),
		);
		expect(request.enabled).toBeUndefined();
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

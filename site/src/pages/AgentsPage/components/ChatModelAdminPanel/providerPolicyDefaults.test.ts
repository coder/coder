import { describe, expect, it } from "vitest";
import {
	normalizeProviderPolicyDefaults,
	type ProviderConfigWithOptionalPolicyFields,
} from "./providerPolicyDefaults";

const baseProviderConfig: ProviderConfigWithOptionalPolicyFields = {
	id: "provider-1",
	provider: "openai",
	display_name: "OpenAI",
	enabled: true,
	has_api_key: true,
	base_url: "https://api.openai.com/v1",
	source: "database",
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-01-01T00:00:00Z",
};

describe("normalizeProviderPolicyDefaults", () => {
	it("passes through explicit policy fields unchanged", () => {
		const providerConfig: ProviderConfigWithOptionalPolicyFields = {
			...baseProviderConfig,
			central_api_key_enabled: false,
			allow_user_api_key: true,
			allow_central_api_key_fallback: true,
		};

		expect(normalizeProviderPolicyDefaults(providerConfig)).toEqual(
			providerConfig,
		);
	});

	it("defaults omitted policy fields to the expected values", () => {
		expect(normalizeProviderPolicyDefaults(baseProviderConfig)).toMatchObject({
			central_api_key_enabled: true,
			allow_user_api_key: false,
			allow_central_api_key_fallback: false,
		});
	});

	it("defaults undefined policy fields to the expected values", () => {
		const providerConfig: ProviderConfigWithOptionalPolicyFields = {
			...baseProviderConfig,
			central_api_key_enabled: undefined,
			allow_user_api_key: undefined,
			allow_central_api_key_fallback: undefined,
		};

		expect(normalizeProviderPolicyDefaults(providerConfig)).toMatchObject({
			central_api_key_enabled: true,
			allow_user_api_key: false,
			allow_central_api_key_fallback: false,
		});
	});
});

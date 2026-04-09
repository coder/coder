import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import type { ProviderState } from "./ChatModelAdminPanel";
import { buildModelProviderOptions } from "./modelProviderOptions";

const makeProviderConfig = (
	id: string,
	enabled: boolean,
	displayName: string,
): TypesGen.ChatProviderConfig => ({
	id,
	provider: "openai",
	display_name: displayName,
	enabled,
	has_api_key: true,
	has_effective_api_key: true,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: false,
	base_url: "",
	source: "database",
});

const makeProviderState = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[],
): ProviderState => ({
	provider: "openai",
	label: "OpenAI",
	providerConfig: providerConfigs[0],
	providerConfigs,
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
});

describe("buildModelProviderOptions", () => {
	it("excludes disabled provider configs from attachment options", () => {
		const options = buildModelProviderOptions([
			makeProviderState([
				makeProviderConfig("enabled-config", true, "Enabled"),
				makeProviderConfig("disabled-config", false, "Disabled"),
				makeProviderConfig(
					"00000000-0000-0000-0000-000000000000",
					true,
					"Nil Sentinel",
				),
			]),
		]);

		expect(options).toHaveLength(1);
		expect(options[0]?.configId).toBe("enabled-config");
	});
});

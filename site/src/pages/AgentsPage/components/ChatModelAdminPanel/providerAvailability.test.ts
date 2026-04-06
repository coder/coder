import { describe, expect, it } from "vitest";
import type { ChatProviderConfig } from "#/api/typesGenerated";
import {
	hasEffectiveProviderAPIKey,
	hasEnabledDatabaseProviderAPIKey,
} from "./providerAvailability";

const providerConfig = (
	overrides: Partial<ChatProviderConfig>,
): ChatProviderConfig => ({
	id: overrides.id ?? "provider-config-id",
	provider: overrides.provider ?? "openai",
	display_name: overrides.display_name ?? "OpenAI",
	enabled: overrides.enabled ?? true,
	has_api_key: overrides.has_api_key ?? false,
	has_effective_api_key:
		overrides.has_effective_api_key ?? overrides.has_api_key ?? false,
	central_api_key_enabled: overrides.central_api_key_enabled ?? true,
	allow_user_api_key: overrides.allow_user_api_key ?? false,
	allow_central_api_key_fallback:
		overrides.allow_central_api_key_fallback ?? false,
	base_url: overrides.base_url,
	source: overrides.source ?? "database",
	created_at: overrides.created_at ?? "2025-01-01T00:00:00Z",
	updated_at: overrides.updated_at ?? "2025-01-01T00:00:00Z",
});

describe("hasEnabledDatabaseProviderAPIKey", () => {
	it("returns true when any enabled config in the family stores a key", () => {
		expect(
			hasEnabledDatabaseProviderAPIKey([
				providerConfig({
					id: "openai-primary",
					enabled: true,
					has_api_key: false,
					has_effective_api_key: false,
				}),
				providerConfig({
					id: "openai-secondary",
					enabled: true,
					has_api_key: true,
					has_effective_api_key: true,
				}),
			]),
		).toBe(true);
	});

	it("ignores disabled configs when computing family key availability", () => {
		expect(
			hasEnabledDatabaseProviderAPIKey([
				providerConfig({
					id: "openai-disabled",
					enabled: false,
					has_api_key: true,
					has_effective_api_key: true,
				}),
			]),
		).toBe(false);
	});

	it("accepts env-backed effective keys for enabled database configs", () => {
		expect(
			hasEnabledDatabaseProviderAPIKey([
				providerConfig({
					id: "openai-env-backed",
					enabled: true,
					has_api_key: false,
					has_effective_api_key: true,
				}),
			]),
		).toBe(true);
	});
});

describe("hasEffectiveProviderAPIKey", () => {
	it("returns true when a managed provider config stores the key", () => {
		expect(
			hasEffectiveProviderAPIKey({
				hasManagedAPIKey: true,
				hasCatalogAPIKey: false,
				hasProviderEntryAPIKey: false,
				hasDatabaseProviderConfig: true,
			}),
		).toBe(true);
	});

	it("falls back to catalog availability for database configs", () => {
		expect(
			hasEffectiveProviderAPIKey({
				hasManagedAPIKey: false,
				hasCatalogAPIKey: true,
				hasProviderEntryAPIKey: false,
				hasDatabaseProviderConfig: true,
			}),
		).toBe(true);
	});

	it("uses the provider entry key when no database config exists", () => {
		expect(
			hasEffectiveProviderAPIKey({
				hasManagedAPIKey: false,
				hasCatalogAPIKey: false,
				hasProviderEntryAPIKey: true,
				hasDatabaseProviderConfig: false,
			}),
		).toBe(true);
	});

	it("returns false when no key source is available", () => {
		expect(
			hasEffectiveProviderAPIKey({
				hasManagedAPIKey: false,
				hasCatalogAPIKey: false,
				hasProviderEntryAPIKey: false,
				hasDatabaseProviderConfig: true,
			}),
		).toBe(false);
	});
});

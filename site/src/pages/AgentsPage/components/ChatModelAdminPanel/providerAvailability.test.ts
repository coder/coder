import { describe, expect, it } from "vitest";
import { hasEffectiveProviderAPIKey } from "./providerAvailability";

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

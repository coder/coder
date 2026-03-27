import { beforeEach, describe, expect, it } from "vitest";
import type { MCPServerConfig } from "#/api/typesGenerated";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	mcpSelectionStorageKey,
	saveMCPSelection,
} from "./MCPServerPicker";

const makeServer = (
	overrides: Partial<MCPServerConfig> & { id: string },
): MCPServerConfig =>
	({
		id: overrides.id,
		display_name: overrides.display_name ?? overrides.id,
		enabled: overrides.enabled ?? true,
		availability: overrides.availability ?? "default_on",
		auth_type: overrides.auth_type ?? "none",
		auth_connected: overrides.auth_connected ?? false,
		icon_url: overrides.icon_url ?? "",
		description: overrides.description ?? "",
		url: overrides.url ?? "",
		transport: overrides.transport ?? "sse",
	}) as MCPServerConfig;

describe("MCP selection persistence", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	describe("saveMCPSelection", () => {
		it("writes a JSON array to localStorage", () => {
			saveMCPSelection(["a", "b"]);
			expect(localStorage.getItem(mcpSelectionStorageKey)).toBe(
				JSON.stringify(["a", "b"]),
			);
		});

		it("writes an empty array when no servers are selected", () => {
			saveMCPSelection([]);
			expect(localStorage.getItem(mcpSelectionStorageKey)).toBe("[]");
		});
	});

	describe("getSavedMCPSelection", () => {
		const servers = [
			makeServer({ id: "s1", availability: "force_on" }),
			makeServer({ id: "s2", availability: "default_on" }),
			makeServer({ id: "s3", availability: "default_off" }),
		];

		it("returns null when nothing is stored", () => {
			expect(getSavedMCPSelection(servers)).toBeNull();
		});

		it("returns null when the server list is empty", () => {
			saveMCPSelection(["s1", "s2"]);
			expect(getSavedMCPSelection([])).toBeNull();
		});

		it("returns null for invalid JSON", () => {
			localStorage.setItem(mcpSelectionStorageKey, "not-json");
			expect(getSavedMCPSelection(servers)).toBeNull();
		});

		it("returns null when stored value is not an array", () => {
			localStorage.setItem(mcpSelectionStorageKey, '"a string"');
			expect(getSavedMCPSelection(servers)).toBeNull();
		});

		it("restores saved IDs that still exist as enabled servers", () => {
			saveMCPSelection(["s2", "s3"]);
			const result = getSavedMCPSelection(servers);
			expect(result).toContain("s2");
			expect(result).toContain("s3");
		});

		it("filters out IDs for servers that no longer exist", () => {
			saveMCPSelection(["s2", "deleted-server"]);
			const result = getSavedMCPSelection(servers);
			expect(result).toContain("s2");
			expect(result).not.toContain("deleted-server");
		});

		it("filters out IDs for disabled servers", () => {
			const withDisabled = [
				...servers,
				makeServer({ id: "s4", enabled: false }),
			];
			saveMCPSelection(["s2", "s4"]);
			const result = getSavedMCPSelection(withDisabled);
			expect(result).toContain("s2");
			expect(result).not.toContain("s4");
		});

		it("always includes force_on servers even if not in saved list", () => {
			saveMCPSelection(["s3"]);
			const result = getSavedMCPSelection(servers);
			expect(result).toContain("s1");
			expect(result).toContain("s3");
		});

		it("does not duplicate force_on servers already in saved list", () => {
			saveMCPSelection(["s1", "s3"]);
			const result = getSavedMCPSelection(servers)!;
			const s1Count = result.filter((id) => id === "s1").length;
			expect(s1Count).toBe(1);
		});

		it("returns an empty selection (plus force_on) when user opted out", () => {
			saveMCPSelection([]);
			const result = getSavedMCPSelection(servers);
			// Only force_on should be present.
			expect(result).toEqual(["s1"]);
		});
	});

	describe("getDefaultMCPSelection", () => {
		it("includes force_on and default_on, excludes default_off", () => {
			const servers = [
				makeServer({ id: "a", availability: "force_on" }),
				makeServer({ id: "b", availability: "default_on" }),
				makeServer({ id: "c", availability: "default_off" }),
			];
			expect(getDefaultMCPSelection(servers)).toEqual(["a", "b"]);
		});

		it("excludes disabled servers", () => {
			const servers = [
				makeServer({ id: "a", availability: "default_on", enabled: false }),
			];
			expect(getDefaultMCPSelection(servers)).toEqual([]);
		});
	});
});

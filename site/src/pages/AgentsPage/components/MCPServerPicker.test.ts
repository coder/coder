import { beforeEach, describe, expect, it } from "vitest";
import type { MCPServerConfig } from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	getDefaultMCPSelection,
	getSavedMCPSelection,
	mcpSelectionStorageKey,
	saveMCPSelection,
} from "./MCPServerPicker";

const buildServer = (
	overrides: Partial<MCPServerConfig> & { id: string },
): MCPServerConfig => ({
	...MockMCPServerConfig,
	display_name: overrides.id,
	transport: "sse",
	url: "",
	...overrides,
});

const organizationId = "organization-1";

describe("MCP selection persistence", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	describe("saveMCPSelection", () => {
		it("writes a JSON array to localStorage", () => {
			saveMCPSelection(organizationId, ["a", "b"]);
			expect(localStorage.getItem(mcpSelectionStorageKey(organizationId))).toBe(
				JSON.stringify(["a", "b"]),
			);
		});

		it("writes an empty array when no servers are selected", () => {
			saveMCPSelection(organizationId, []);
			expect(localStorage.getItem(mcpSelectionStorageKey(organizationId))).toBe(
				"[]",
			);
		});

		it("keeps selections separate by organization", () => {
			const otherOrganizationId = "organization-2";
			saveMCPSelection(organizationId, ["a"]);
			saveMCPSelection(otherOrganizationId, ["b"]);

			expect(localStorage.getItem(mcpSelectionStorageKey(organizationId))).toBe(
				JSON.stringify(["a"]),
			);
			expect(
				localStorage.getItem(mcpSelectionStorageKey(otherOrganizationId)),
			).toBe(JSON.stringify(["b"]));
		});
	});

	describe("getSavedMCPSelection", () => {
		const servers = [
			buildServer({ id: "s1", availability: "force_on" }),
			buildServer({ id: "s2", availability: "default_on" }),
			buildServer({ id: "s3", availability: "default_off" }),
		];

		it("returns null when nothing is stored", () => {
			expect(getSavedMCPSelection(organizationId, servers)).toBeNull();
		});

		it("returns null when the server list is empty", () => {
			saveMCPSelection(organizationId, ["s1", "s2"]);
			expect(getSavedMCPSelection(organizationId, [])).toBeNull();
		});

		it("returns null for invalid JSON", () => {
			localStorage.setItem(mcpSelectionStorageKey(organizationId), "not-json");
			expect(getSavedMCPSelection(organizationId, servers)).toBeNull();
		});

		it("returns null when stored value is not an array", () => {
			localStorage.setItem(
				mcpSelectionStorageKey(organizationId),
				'"a string"',
			);
			expect(getSavedMCPSelection(organizationId, servers)).toBeNull();
		});

		it("heals a legacy selection into the organization-scoped key on read", () => {
			localStorage.setItem(
				"agents.selected-mcp-server-ids",
				JSON.stringify(["s3"]),
			);

			expect(getSavedMCPSelection(organizationId, servers, true)).toEqual([
				"s3",
				"s1",
			]);
			expect(localStorage.getItem(mcpSelectionStorageKey(organizationId))).toBe(
				JSON.stringify(["s3", "s1"]),
			);
			expect(localStorage.getItem("agents.selected-mcp-server-ids")).toBeNull();
			// Subsequent reads come from the scoped key.
			expect(getSavedMCPSelection(organizationId, servers)).toEqual([
				"s3",
				"s1",
			]);
		});

		it("heals an empty legacy selection without enabling default-on servers", () => {
			localStorage.setItem("agents.selected-mcp-server-ids", "[]");

			expect(getSavedMCPSelection(organizationId, servers, true)).toEqual([
				"s1",
			]);

			expect(localStorage.getItem(mcpSelectionStorageKey(organizationId))).toBe(
				JSON.stringify(["s1"]),
			);
			expect(localStorage.getItem("agents.selected-mcp-server-ids")).toBeNull();
		});

		it("leaves the legacy selection alone while the server list is empty", () => {
			localStorage.setItem(
				"agents.selected-mcp-server-ids",
				JSON.stringify(["s3"]),
			);

			expect(getSavedMCPSelection(organizationId, [], true)).toBeNull();

			expect(
				localStorage.getItem(mcpSelectionStorageKey(organizationId)),
			).toBeNull();
			expect(localStorage.getItem("agents.selected-mcp-server-ids")).toBe(
				JSON.stringify(["s3"]),
			);
		});

		it("does not read or heal the legacy selection for non-default organizations", () => {
			localStorage.setItem(
				"agents.selected-mcp-server-ids",
				JSON.stringify(["s3"]),
			);

			expect(getSavedMCPSelection(organizationId, servers)).toBeNull();

			expect(
				localStorage.getItem(mcpSelectionStorageKey(organizationId)),
			).toBeNull();
			expect(localStorage.getItem("agents.selected-mcp-server-ids")).toBe(
				JSON.stringify(["s3"]),
			);
		});

		it("prefers the scoped key over a lingering legacy selection", () => {
			saveMCPSelection(organizationId, ["s2"]);
			localStorage.setItem(
				"agents.selected-mcp-server-ids",
				JSON.stringify(["s3"]),
			);

			expect(getSavedMCPSelection(organizationId, servers, true)).toEqual([
				"s2",
				"s1",
			]);
			expect(localStorage.getItem("agents.selected-mcp-server-ids")).toBe(
				JSON.stringify(["s3"]),
			);
		});

		it("restores saved IDs that still exist as enabled servers", () => {
			saveMCPSelection(organizationId, ["s2", "s3"]);
			const result = getSavedMCPSelection(organizationId, servers);
			expect(result).toContain("s2");
			expect(result).toContain("s3");
		});

		it("filters out IDs for servers that no longer exist", () => {
			saveMCPSelection(organizationId, ["s2", "deleted-server"]);
			const result = getSavedMCPSelection(organizationId, servers);
			expect(result).toContain("s2");
			expect(result).not.toContain("deleted-server");
		});

		it("filters out IDs for disabled servers", () => {
			const withDisabled = [
				...servers,
				buildServer({ id: "s4", enabled: false }),
			];
			saveMCPSelection(organizationId, ["s2", "s4"]);
			const result = getSavedMCPSelection(organizationId, withDisabled);
			expect(result).toContain("s2");
			expect(result).not.toContain("s4");
		});

		it("always includes force_on servers even if not in saved list", () => {
			saveMCPSelection(organizationId, ["s3"]);
			const result = getSavedMCPSelection(organizationId, servers);
			expect(result).toContain("s1");
			expect(result).toContain("s3");
		});

		it("does not duplicate force_on servers already in saved list", () => {
			saveMCPSelection(organizationId, ["s1", "s3"]);
			const result = getSavedMCPSelection(organizationId, servers) ?? [];
			const s1Count = result.filter((id) => id === "s1").length;
			expect(s1Count).toBe(1);
		});

		it("returns an empty selection (plus force_on) when user opted out", () => {
			saveMCPSelection(organizationId, []);
			const result = getSavedMCPSelection(organizationId, servers);
			// Only force_on should be present.
			expect(result).toEqual(["s1"]);
		});
	});

	describe("getDefaultMCPSelection", () => {
		it("includes force_on and default_on, excludes default_off", () => {
			const servers = [
				buildServer({ id: "a", availability: "force_on" }),
				buildServer({ id: "b", availability: "default_on" }),
				buildServer({ id: "c", availability: "default_off" }),
			];
			expect(getDefaultMCPSelection(servers)).toEqual(["a", "b"]);
		});

		it("excludes disabled servers", () => {
			const servers = [
				buildServer({ id: "a", availability: "default_on", enabled: false }),
			];
			expect(getDefaultMCPSelection(servers)).toEqual([]);
		});
	});
});

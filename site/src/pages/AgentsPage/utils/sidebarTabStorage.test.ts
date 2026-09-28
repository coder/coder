import { beforeEach, describe, expect, it } from "vitest";
import {
	clearPersistedSidebarTabId,
	getPersistedSidebarTabId,
	lastActiveSidebarTabStorageKeyPrefix,
	savePersistedSidebarTabId,
} from "./sidebarTabStorage";

describe("sidebar tab persistence", () => {
	beforeEach(() => {
		localStorage.clear();
	});

	describe("getPersistedSidebarTabId", () => {
		it("returns null when no value is stored for that chat", () => {
			expect(getPersistedSidebarTabId("chat-1")).toBeNull();
		});

		it("returns the stored string when one is present", () => {
			localStorage.setItem(
				`${lastActiveSidebarTabStorageKeyPrefix}chat-1`,
				"terminal",
			);
			expect(getPersistedSidebarTabId("chat-1")).toBe("terminal");
		});

		it("returns null when chatID is undefined", () => {
			expect(getPersistedSidebarTabId(undefined)).toBeNull();
		});

		it("returns null when chatID is empty string", () => {
			expect(getPersistedSidebarTabId("")).toBeNull();
		});

		it("reads from the key agents.last-active-tab.<chatID>", () => {
			const chatID = "chat-xyz";
			localStorage.setItem(`agents.last-active-tab.${chatID}`, "git");
			expect(getPersistedSidebarTabId(chatID)).toBe("git");
		});
	});

	describe("savePersistedSidebarTabId", () => {
		it("writes tabID to agents.last-active-tab.<chatID>", () => {
			savePersistedSidebarTabId("chat-1", "desktop");
			expect(
				localStorage.getItem(`${lastActiveSidebarTabStorageKeyPrefix}chat-1`),
			).toBe("desktop");
		});

		it("is a no-op when chatID is undefined", () => {
			savePersistedSidebarTabId(undefined, "desktop");
			expect(localStorage.length).toBe(0);
		});

		it("is a no-op when chatID is empty string", () => {
			savePersistedSidebarTabId("", "desktop");
			expect(localStorage.length).toBe(0);
		});

		it("can be round-tripped with getPersistedSidebarTabId", () => {
			savePersistedSidebarTabId("chat-rt", "terminal");
			expect(getPersistedSidebarTabId("chat-rt")).toBe("terminal");
		});

		it("does not collide across different chatIDs", () => {
			savePersistedSidebarTabId("chat-a", "git");
			savePersistedSidebarTabId("chat-b", "desktop");
			expect(getPersistedSidebarTabId("chat-a")).toBe("git");
			expect(getPersistedSidebarTabId("chat-b")).toBe("desktop");
		});
	});

	describe("clearPersistedSidebarTabId", () => {
		it("removes agents.last-active-tab.<chatID> from storage", () => {
			savePersistedSidebarTabId("chat-1", "terminal");
			clearPersistedSidebarTabId("chat-1");
			expect(getPersistedSidebarTabId("chat-1")).toBeNull();
		});

		it("is a no-op when nothing is stored", () => {
			// Calling twice should not throw.
			clearPersistedSidebarTabId("chat-1");
			clearPersistedSidebarTabId("chat-1");
			expect(getPersistedSidebarTabId("chat-1")).toBeNull();
		});

		it("is a no-op when chatID is undefined", () => {
			savePersistedSidebarTabId("chat-1", "git");
			clearPersistedSidebarTabId(undefined);
			expect(getPersistedSidebarTabId("chat-1")).toBe("git");
		});

		it("is a no-op when chatID is empty string", () => {
			savePersistedSidebarTabId("chat-1", "git");
			clearPersistedSidebarTabId("");
			expect(getPersistedSidebarTabId("chat-1")).toBe("git");
		});

		it("only affects the target chat's entry", () => {
			savePersistedSidebarTabId("chat-a", "git");
			savePersistedSidebarTabId("chat-b", "desktop");
			clearPersistedSidebarTabId("chat-a");
			expect(getPersistedSidebarTabId("chat-a")).toBeNull();
			expect(getPersistedSidebarTabId("chat-b")).toBe("desktop");
		});
	});
});

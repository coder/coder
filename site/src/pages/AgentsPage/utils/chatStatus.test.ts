import { describe, expect, it } from "vitest";
import { ChatStatuses } from "#/api/typesGenerated";
import { isChatTurnActive } from "./chatStatus";

describe("isChatTurnActive", () => {
	it.each(ChatStatuses)("classifies %s", (status) => {
		const expected =
			status === "running" ||
			status === "requires_action" ||
			status === "interrupting";
		expect(isChatTurnActive(status)).toBe(expected);
	});

	it("treats a missing status as idle", () => {
		expect(isChatTurnActive(null)).toBe(false);
		expect(isChatTurnActive(undefined)).toBe(false);
	});
});

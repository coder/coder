import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { getArchiveBlockedReason } from "./ChatActionsMenuItems";

const childWithStatus = (status: TypesGen.ChatStatus): TypesGen.Chat => ({
	...MockChat,
	id: `child-${status}`,
	status,
});

describe("getArchiveBlockedReason", () => {
	it("returns undefined when the whole family is waiting", () => {
		expect(
			getArchiveBlockedReason("waiting", [childWithStatus("waiting")]),
		).toBeUndefined();
	});

	it("returns paused for a paused child on a waiting root", () => {
		expect(
			getArchiveBlockedReason("waiting", [childWithStatus("paused")]),
		).toBe("paused");
	});

	it("prefers active over paused", () => {
		expect(
			getArchiveBlockedReason("paused", [childWithStatus("running")]),
		).toBe("active");
		expect(
			getArchiveBlockedReason("running", [childWithStatus("paused")]),
		).toBe("active");
	});
});

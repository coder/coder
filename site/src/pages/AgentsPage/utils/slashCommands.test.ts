import { describe, expect, it } from "vitest";
import {
	COMPACT_SLASH_COMMAND,
	resolveChatSlashCommandAvailability,
} from "./slashCommands";

describe("resolveChatSlashCommandAvailability", () => {
	it("stays pending until both skill sources resolve", () => {
		expect(
			resolveChatSlashCommandAvailability(COMPACT_SLASH_COMMAND, undefined, []),
		).toBe("pending");
		expect(
			resolveChatSlashCommandAvailability(COMPACT_SLASH_COMMAND, [], undefined),
		).toBe("pending");
	});

	it("is unavailable when either skill source defines the command", () => {
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				[{ name: "compact" }],
				[],
			),
		).toBe("unavailable");
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				[],
				[{ name: "compact" }],
			),
		).toBe("unavailable");
	});

	it("is available when both skill sources resolve without a collision", () => {
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				[{ name: "review" }],
				[{ name: "test" }],
			),
		).toBe("available");
	});
});

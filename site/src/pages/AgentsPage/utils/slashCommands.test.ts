import { describe, expect, it } from "vitest";
import {
	CLEAR_SLASH_COMMAND,
	COMPACT_SLASH_COMMAND,
	resolveChatSlashCommandAvailability,
	runtimeSlashCommands,
} from "./slashCommands";

describe("runtimeSlashCommands", () => {
	it("maps advertised commands and drops empty input hints", () => {
		expect(
			runtimeSlashCommands([
				{ name: "review", description: "Review the diff", input_hint: "" },
				{ name: "model", description: "Switch model", input_hint: "<name>" },
			]),
		).toEqual([
			{ name: "review", description: "Review the diff", inputHint: undefined },
			{ name: "model", description: "Switch model", inputHint: "<name>" },
		]);
	});

	it("returns nothing for chats without advertised commands", () => {
		expect(runtimeSlashCommands(undefined)).toEqual([]);
	});
});

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

	it("resolves clear availability and skill collisions", () => {
		expect(
			resolveChatSlashCommandAvailability(CLEAR_SLASH_COMMAND, undefined, []),
		).toBe("pending");
		expect(
			resolveChatSlashCommandAvailability(
				CLEAR_SLASH_COMMAND,
				[{ name: "clear" }],
				[],
			),
		).toBe("unavailable");
		expect(
			resolveChatSlashCommandAvailability(
				CLEAR_SLASH_COMMAND,
				[{ name: "review" }],
				[{ name: "test" }],
			),
		).toBe("available");
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

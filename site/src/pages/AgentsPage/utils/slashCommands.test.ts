import { describe, expect, it } from "vitest";
import type { SkillSourceList } from "./skillAliases";
import {
	CLEAR_SLASH_COMMAND,
	COMPACT_SLASH_COMMAND,
	resolveChatSlashCommandAvailability,
} from "./slashCommands";

const skillLists = (
	personal: readonly { name: string }[] | undefined,
	org: readonly { name: string }[] | undefined,
	workspace: readonly { name: string }[] | undefined,
): SkillSourceList<{ name: string }>[] => [
	{ source: "personal", skills: personal },
	{ source: "org", skills: org },
	{ source: "workspace", skills: workspace },
];

describe("resolveChatSlashCommandAvailability", () => {
	it("stays pending until every skill source resolves", () => {
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists(undefined, [], []),
			),
		).toBe("pending");
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists([], undefined, []),
			),
		).toBe("pending");
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists([], [], undefined),
			),
		).toBe("pending");
	});

	it("is unavailable when any skill source defines the command", () => {
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists([{ name: "compact" }], [], []),
			),
		).toBe("unavailable");
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists([], [{ name: "compact" }], []),
			),
		).toBe("unavailable");
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists([], [], [{ name: "compact" }]),
			),
		).toBe("unavailable");
	});

	it("resolves clear availability and skill collisions", () => {
		expect(
			resolveChatSlashCommandAvailability(
				CLEAR_SLASH_COMMAND,
				skillLists(undefined, [], []),
			),
		).toBe("pending");
		expect(
			resolveChatSlashCommandAvailability(
				CLEAR_SLASH_COMMAND,
				skillLists([], [{ name: "clear" }], []),
			),
		).toBe("unavailable");
		expect(
			resolveChatSlashCommandAvailability(
				CLEAR_SLASH_COMMAND,
				skillLists(
					[{ name: "review" }],
					[{ name: "lint" }],
					[{ name: "test" }],
				),
			),
		).toBe("available");
	});

	it("is available when every skill source resolves without a collision", () => {
		expect(
			resolveChatSlashCommandAvailability(
				COMPACT_SLASH_COMMAND,
				skillLists(
					[{ name: "review" }],
					[{ name: "lint" }],
					[{ name: "test" }],
				),
			),
		).toBe("available");
	});
});

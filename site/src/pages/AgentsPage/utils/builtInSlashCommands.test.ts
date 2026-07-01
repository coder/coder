import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	filterBuiltInSlashCommands,
	parseBuiltInSlashCommand,
} from "./builtInSlashCommands";

const textPart = (text: string): TypesGen.ChatInputPart => ({
	type: "text",
	text,
});

describe("parseBuiltInSlashCommand", () => {
	it("accepts exact compact command with surrounding whitespace", () => {
		expect(
			parseBuiltInSlashCommand({
				message: "  /compact  ",
				content: [textPart("  /compact  ")],
			}),
		).toBe("compact");
	});

	it("rejects non-exact compact input", () => {
		for (const message of ["", "/comp", "/compact please", "please /compact"]) {
			expect(
				parseBuiltInSlashCommand({
					message,
					content: [textPart(message)],
				}),
			).toBeNull();
		}
	});

	it("rejects edits, attachments, and non-text content", () => {
		expect(
			parseBuiltInSlashCommand({
				message: "/compact",
				content: [textPart("/compact")],
				isEditing: true,
			}),
		).toBeNull();
		expect(
			parseBuiltInSlashCommand({
				message: "/compact",
				content: [textPart("/compact")],
				hasAttachments: true,
			}),
		).toBeNull();
		expect(
			parseBuiltInSlashCommand({
				message: "/compact",
				content: [textPart("/compact"), { type: "file", file_id: "file-1" }],
			}),
		).toBeNull();
		expect(
			parseBuiltInSlashCommand({
				message: "/compact",
				content: [
					textPart("/compact"),
					{
						type: "file-reference",
						file_name: "main.go",
						start_line: 1,
						end_line: 2,
						content: "package main",
					},
				],
			}),
		).toBeNull();
	});
});

describe("filterBuiltInSlashCommands", () => {
	it("returns all commands for an empty query", () => {
		expect(filterBuiltInSlashCommands("").map((command) => command.id)).toEqual(
			["compact"],
		);
	});

	it("matches by name prefix and description", () => {
		expect(
			filterBuiltInSlashCommands("comp").map((command) => command.id),
		).toEqual(["compact"]);
		expect(
			filterBuiltInSlashCommands("summarize").map((command) => command.id),
		).toEqual(["compact"]);
	});

	it("returns an empty list when no command matches", () => {
		expect(filterBuiltInSlashCommands("deploy")).toEqual([]);
	});
});

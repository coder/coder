import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { createChatStore } from "../components/ChatConversation/chatStore";
import {
	parseEditedFiles,
	selectEditedFilesThisTurn,
	sourceFileWasEdited,
} from "./editedFiles";

const message = (
	id: number,
	role: "user" | "assistant",
	content: unknown[],
): TypesGen.ChatMessage =>
	({
		id,
		chat_id: "chat-1",
		created_at: `2025-01-01T00:00:${String(id).padStart(2, "0")}.000Z`,
		role,
		content,
	}) as TypesGen.ChatMessage;

const editFiles = (...paths: string[]) => ({
	type: "tool-call",
	tool_name: "edit_files",
	args: {
		files: paths.map((path) => ({
			path,
			edits: [{ old_text: "a", new_text: "b" }],
		})),
	},
});

describe("selectEditedFilesThisTurn", () => {
	it("collects paths from edit_files and write_file calls since the last user message", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "earlier" }]),
			message(2, "assistant", [editFiles("/repo/src/Old.tsx")]),
			message(3, "user", [{ type: "text", text: "make it red" }]),
			message(4, "assistant", [
				{ type: "text", text: "Sure." },
				editFiles("/repo/src/Button.tsx", "/repo/src/theme.css"),
				{
					type: "tool-call",
					tool_name: "write_file",
					args: { path: "/repo/src/New.tsx", content: "" },
				},
				{
					type: "tool-call",
					tool_name: "execute",
					args: { command: "pnpm test" },
				},
			]),
		]);
		expect(
			parseEditedFiles(selectEditedFilesThisTurn(store.getSnapshot())),
		).toEqual([
			"/repo/src/Button.tsx",
			"/repo/src/theme.css",
			"/repo/src/New.tsx",
		]);
	});

	it("includes calls that are still streaming", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "make it red" }]),
		]);
		store.setStreamState({
			blocks: [],
			toolCalls: {
				call: {
					id: "call",
					name: "write_file",
					args: { path: "/repo/src/Live.tsx" },
				},
				raw: {
					id: "raw",
					name: "write_file",
					argsRaw: '{"path":"/repo/src/Raw.tsx","content":"x"}',
				},
			},
			toolResults: {},
			sources: [],
		});
		expect(
			parseEditedFiles(selectEditedFilesThisTurn(store.getSnapshot())),
		).toEqual(["/repo/src/Live.tsx", "/repo/src/Raw.tsx"]);
	});

	it("is empty with no edits", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "hello" }]),
			message(2, "assistant", [{ type: "text", text: "hi" }]),
		]);
		expect(selectEditedFilesThisTurn(store.getSnapshot())).toBe("");
		expect(parseEditedFiles("")).toEqual([]);
	});
});

describe("sourceFileWasEdited", () => {
	const edited = ["/home/coder/app/src/components/Button.tsx"];

	it("matches the same file with a line and column", () => {
		expect(
			sourceFileWasEdited(
				"/home/coder/app/src/components/Button.tsx:12:5",
				edited,
			),
		).toBe(true);
	});

	it("matches across different roots by trailing segments", () => {
		expect(
			sourceFileWasEdited("/app/src/components/Button.tsx:3", edited),
		).toBe(true);
		expect(sourceFileWasEdited("src/components/Button.tsx", edited)).toBe(true);
		expect(
			sourceFileWasEdited("webpack:///./src/components/Button.tsx:3", edited),
		).toBe(true);
		expect(
			sourceFileWasEdited("/home/coder/app/src/components/Button.tsx", [
				"src/components/Button.tsx",
			]),
		).toBe(true);
	});

	it("does not match on the basename alone", () => {
		expect(sourceFileWasEdited("/app/src/ui/Button.tsx:3", edited)).toBe(false);
		expect(sourceFileWasEdited("Button.tsx", edited)).toBe(false);
		expect(
			sourceFileWasEdited("/app/src/pages/Foo/index.tsx", [
				"/app/src/pages/Bar/index.tsx",
			]),
		).toBe(false);
	});

	it("does not match other files or missing locations", () => {
		expect(sourceFileWasEdited("/home/coder/app/src/App.tsx:1", edited)).toBe(
			false,
		);
		expect(sourceFileWasEdited(undefined, edited)).toBe(false);
		expect(sourceFileWasEdited("", edited)).toBe(false);
		expect(sourceFileWasEdited("/app/src/components/Button.tsx", [])).toBe(
			false,
		);
	});
});

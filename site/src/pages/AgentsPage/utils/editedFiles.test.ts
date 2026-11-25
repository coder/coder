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
	role: "user" | "assistant" | "tool",
	content: unknown[],
): TypesGen.ChatMessage =>
	({
		id,
		chat_id: "chat-1",
		created_at: `2025-01-01T00:00:${String(id).padStart(2, "0")}.000Z`,
		role,
		content,
	}) as TypesGen.ChatMessage;

const editFiles = (id: string, paths: string[], serialized = false) => {
	const files = paths.map((path) => ({
		path,
		edits: [{ old_text: "a", new_text: "b" }],
	}));
	return {
		type: "tool-call",
		tool_call_id: id,
		tool_name: "edit_files",
		args: { files: serialized ? JSON.stringify(files) : files },
	};
};

const result = (
	id: string,
	name: "edit_files" | "write_file",
	value: unknown,
	isError = false,
) => ({
	type: "tool-result",
	tool_call_id: id,
	tool_name: name,
	result: value,
	is_error: isError,
});

describe("selectEditedFilesThisTurn", () => {
	it("collects successful edits in this turn from results, with both files-arg encodings", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "earlier" }]),
			message(2, "assistant", [editFiles("old", ["/repo/src/Old.tsx"])]),
			message(3, "tool", [
				result("old", "edit_files", {
					ok: true,
					files: [{ path: "/repo/src/Old.tsx", diff: "+old" }],
				}),
			]),
			message(4, "user", [{ type: "text", text: "make it red" }]),
			message(5, "assistant", [
				{ type: "text", text: "Sure." },
				editFiles("edit-array", ["/container/src/Button.tsx"]),
				editFiles("edit-string", ["/container/src/theme.css"], true),
				{
					type: "tool-call",
					tool_call_id: "write",
					tool_name: "write_file",
					args: { path: "/repo/src/New.tsx", content: "new file" },
				},
				{
					type: "tool-call",
					tool_name: "execute",
					args: { command: "pnpm test" },
				},
			]),
			message(6, "tool", [
				result("edit-array", "edit_files", {
					ok: true,
					files: [
						{ path: "/repo/src/Button.tsx", diff: "@@ -1 +1 @@\n-a\n+b" },
					],
				}),
				result("edit-string", "edit_files", {
					ok: true,
					files: [{ path: "/repo/src/theme.css", diff: "@@ -1 +1 @@\n-a\n+b" }],
				}),
				result("write", "write_file", { ok: true }),
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

	it("does not acknowledge failed, incomplete, malformed, or no-op calls", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "update this" }]),
			message(2, "assistant", [
				editFiles("failed", ["/repo/src/Failed.tsx"]),
				editFiles("no-op", ["/repo/src/Unchanged.tsx"]),
				editFiles("malformed", ["/repo/src/Malformed.tsx"]),
				editFiles("malformed-entry", ["/repo/src/BadEntry.tsx"]),
				editFiles("incomplete", ["/repo/src/Incomplete.tsx"]),
				{
					type: "tool-call",
					tool_call_id: "write-failed",
					tool_name: "write_file",
					args: { path: "/repo/src/FailedWrite.tsx", content: "new" },
				},
			]),
			message(3, "tool", [
				result("failed", "edit_files", { error: "old_text not found" }, true),
				result("no-op", "edit_files", {
					ok: true,
					files: [{ path: "/repo/src/Unchanged.tsx", diff: "" }],
				}),
				result("malformed", "edit_files", { ok: true, files: "bad" }),
				result("malformed-entry", "edit_files", {
					ok: true,
					files: [
						{ path: 42, diff: "+x" },
						{ path: "/repo/src/BadEntry.tsx", diff: 42 },
					],
				}),
				result(
					"write-failed",
					"write_file",
					{ error: "permission denied" },
					true,
				),
			]),
		]);
		expect(selectEditedFilesThisTurn(store.getSnapshot())).toBe("");
	});

	it("acknowledges only completed successful streaming results", () => {
		const store = createChatStore();
		store.replaceMessages([
			message(1, "user", [{ type: "text", text: "make it red" }]),
		]);
		store.setStreamState({
			blocks: [],
			toolCalls: {
				edit: { id: "edit", name: "edit_files", args: { files: "[]" } },
				write: {
					id: "write",
					name: "write_file",
					argsRaw: '{"path":"/repo/src/Live.tsx","content":"x"}',
				},
				pending: {
					id: "pending",
					name: "write_file",
					args: { path: "/repo/src/Pending.tsx" },
				},
				failed: {
					id: "failed",
					name: "write_file",
					args: { path: "/repo/src/Failed.tsx" },
				},
			},
			toolResults: {
				edit: {
					id: "edit",
					name: "edit_files",
					result: {
						ok: true,
						files: [{ path: "/repo/src/App.tsx", diff: "@@ -0,0 +1 @@\n+x" }],
					},
					isError: false,
				},
				write: {
					id: "write",
					name: "write_file",
					resultRaw: '{"ok":true}',
					isError: false,
				},
				pending: {
					id: "pending",
					name: "write_file",
					result: { ok: true },
					isError: false,
					isStreaming: true,
				},
				failed: {
					id: "failed",
					name: "write_file",
					result: { error: "denied" },
					isError: true,
				},
			},
			sources: [],
		});
		expect(
			parseEditedFiles(selectEditedFilesThisTurn(store.getSnapshot())),
		).toEqual(["/repo/src/App.tsx", "/repo/src/Live.tsx"]);
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

	it("tolerates a location that is not a string", () => {
		// What `sources[id]` yields for an id like "constructor" on a plain
		// object: inherited, truthy, and not a path.
		const sources: Record<string, string> = {};
		const inherited = sources.constructor as unknown as string;
		expect(sourceFileWasEdited(inherited, edited)).toBe(false);
	});
});

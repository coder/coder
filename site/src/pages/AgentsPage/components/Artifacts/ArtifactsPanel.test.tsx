import { act, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChatMessage, ChatMessagePart } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { MockChatArtifactVersion } from "#/testHelpers/chatEntities";
import { ArtifactToolCard } from "../ChatElements/tools/ArtifactToolCard";
import { ArtifactsPanelContext } from "./ArtifactsPanelContext";
import {
	ARTIFACT_PROMPT_TYPE,
	acceptArtifactMessage,
	artifactPromptAction,
	useArtifactPromptHandler,
} from "./artifactBridge";
import {
	deriveArtifactIds,
	deriveSeenArtifactVersions,
	describeArtifactVersion,
	kindOf,
} from "./artifactUtils";

const toolResult = (
	toolName: string,
	result: unknown,
	isError = false,
): ChatMessagePart => ({
	type: "tool-result",
	tool_name: toolName,
	result,
	is_error: isError,
});

const message = (id: number, content: ChatMessagePart[]): ChatMessage => ({
	id,
	chat_id: "chat-1",
	created_at: "2026-10-25T12:00:00Z",
	role: "tool",
	content,
});

const transcript: ChatMessage[] = [
	message(1, [
		toolResult("publish_artifact", { artifact_id: "a", version: 1 }),
	]),
	message(2, [
		toolResult("read_artifact", { artifact_id: "b", version: 3 }),
		toolResult("execute", { artifact_id: "not-an-artifact" }),
	]),
	message(3, [
		toolResult("publish_artifact", { error: "conflict" }, true),
		toolResult("publish_artifact", { artifact_id: "error", version: 9 }, true),
		toolResult("publish_artifact", { ok: true }),
	]),
	message(4, [
		toolResult("publish_artifact", { artifact_id: "a", version: 2 }),
	]),
];

describe("deriveArtifactIds", () => {
	it("returns unique artifact IDs from successful artifact tool results, newest first", () => {
		expect(deriveArtifactIds(transcript)).toEqual(["b", "a"]);
	});

	it("returns no IDs for a chat without artifact tool results", () => {
		expect(deriveArtifactIds([message(1, [])])).toEqual([]);
	});
});

describe("deriveSeenArtifactVersions", () => {
	it("keeps the highest version that a tool result names", () => {
		expect(deriveSeenArtifactVersions(transcript)).toEqual(
			new Map([
				["a", 2],
				["b", 3],
			]),
		);
	});
});

describe("kindOf", () => {
	it.each([
		["report.html", "text/html", "html"],
		["REPORT.HTM", "text/html", "html"],
		["flow.mmd", "text/plain", "mermaid"],
		["flow.mermaid", "text/plain", "mermaid"],
		["notes.md", "text/plain", "markdown"],
		["notes", "text/markdown", "markdown"],
		["chart.svg", "image/svg+xml", "image"],
		["shot.png", "image/png", "image"],
		["data.json", "application/json", "text"],
		["table.csv", "text/csv", "text"],
		["doc.pdf", "application/pdf", "download"],
		["archive", "application/octet-stream", "download"],
	])("maps %s (%s) to %s", (name, mediaType, kind) => {
		expect(kindOf(name, mediaType)).toBe(kind);
	});
});

describe("describeArtifactVersion", () => {
	const mockArtifactVersion = {
		...MockChatArtifactVersion,
		label: "",
		created_by_username: "alice",
		chat_id: "chat-1",
		chat_title: "",
	};

	it.each([
		["this chat", mockArtifactVersion, "chat-1", /^v1 · this chat · /],
		[
			"the viewer's chat",
			{ ...mockArtifactVersion, chat_title: "Repo report" },
			undefined,
			/^v1 · Repo report · /,
		],
		[
			"another user's chat by its author",
			mockArtifactVersion,
			undefined,
			/^v1 · by alice · /,
		],
		[
			"an unknown author as another chat",
			{ ...mockArtifactVersion, created_by_username: "" },
			undefined,
			/^v1 · another chat · /,
		],
	])("names %s", (_, v, currentChatId, expected) => {
		expect(describeArtifactVersion(v, currentChatId)).toMatch(expected);
	});
});

describe("acceptArtifactMessage", () => {
	const setup = () => {
		const frame = document.createElement("iframe");
		document.body.appendChild(frame);
		const otherFrame = document.createElement("iframe");
		document.body.appendChild(otherFrame);
		const state = {
			frame,
			navigated: false,
			activeElement: frame,
			userActivationActive: true,
		};
		const event = {
			source: frame.contentWindow,
			data: { type: ARTIFACT_PROMPT_TYPE, text: "Use option B" },
		};
		return { frame, otherFrame, state, event };
	};

	afterEach(() => {
		document.body.innerHTML = "";
	});

	it("accepts a prompt from the focused frame right after a click", () => {
		const { state, event } = setup();
		expect(acceptArtifactMessage(event, state)).toBe("Use option B");
	});

	it("accepts text of exactly 4000 characters", () => {
		const { state, event } = setup();
		const text = "x".repeat(4000);
		expect(
			acceptArtifactMessage({ ...event, data: { ...event.data, text } }, state),
		).toBe(text);
	});

	it("rejects a message from another window", () => {
		const { otherFrame, state, event } = setup();
		expect(
			acceptArtifactMessage(
				{ ...event, source: otherFrame.contentWindow },
				state,
			),
		).toBeUndefined();
	});

	it("rejects a message after the frame navigated", () => {
		const { state, event } = setup();
		expect(
			acceptArtifactMessage(event, { ...state, navigated: true }),
		).toBeUndefined();
	});

	it("rejects a message when the frame does not have focus", () => {
		const { state, event } = setup();
		expect(
			acceptArtifactMessage(event, { ...state, activeElement: document.body }),
		).toBeUndefined();
	});

	it("rejects a message without an active user activation", () => {
		const { state, event } = setup();
		expect(
			acceptArtifactMessage(event, { ...state, userActivationActive: false }),
		).toBeUndefined();
	});

	it("rejects a message when the frame is gone", () => {
		const { state, event } = setup();
		expect(
			acceptArtifactMessage(event, { ...state, frame: null }),
		).toBeUndefined();
	});

	it.each([
		["a wrong type", { type: "coder:other", text: "hi" }],
		["a missing type", { text: "hi" }],
		["a non-string text", { type: ARTIFACT_PROMPT_TYPE, text: 42 }],
		["a missing text", { type: ARTIFACT_PROMPT_TYPE }],
		["an empty text", { type: ARTIFACT_PROMPT_TYPE, text: "" }],
		[
			"a text over 4000 characters",
			{ type: ARTIFACT_PROMPT_TYPE, text: "x".repeat(4001) },
		],
		["a string payload", "coder:artifact-prompt"],
		["a null payload", null],
	])("rejects %s", (_name, data) => {
		const { state, event } = setup();
		expect(acceptArtifactMessage({ ...event, data }, state)).toBeUndefined();
	});
});

describe("artifactPromptAction", () => {
	it("never sends a slash command", () => {
		expect(artifactPromptAction("  /clear", undefined, 0)).toBe("input");
	});
});

describe("useArtifactPromptHandler", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it("sends, then puts a prompt within 5 seconds in the input, then sends again after 5 seconds", () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2026-10-25T12:00:00Z"));
		const send = vi.fn();
		const insert = vi.fn();
		const { result } = renderHook(() =>
			useArtifactPromptHandler({ canSend: true, send, insert }),
		);

		act(() => result.current("first"));
		expect(send).toHaveBeenLastCalledWith("first");

		vi.advanceTimersByTime(4999);
		act(() => result.current("second"));
		expect(insert).toHaveBeenLastCalledWith("second");
		expect(send).toHaveBeenCalledTimes(1);

		// The gate counts from the last sent prompt, not the last inserted one.
		vi.advanceTimersByTime(1);
		act(() => result.current("third"));
		expect(send).toHaveBeenLastCalledWith("third");
		expect(send).toHaveBeenCalledTimes(2);
		expect(insert).toHaveBeenCalledTimes(1);
	});

	it("puts the prompt in the input when the chat cannot send", () => {
		const send = vi.fn();
		const insert = vi.fn();
		const { result } = renderHook(() =>
			useArtifactPromptHandler({ canSend: false, send, insert }),
		);
		act(() => result.current("hello"));
		expect(send).not.toHaveBeenCalled();
		expect(insert).toHaveBeenCalledWith("hello");
	});
});

describe("ArtifactToolCard", () => {
	it("opens the published version in the Artifacts tab", async () => {
		const user = userEvent.setup();
		const onOpenArtifact = vi.fn();
		render(
			<TooltipProvider>
				<ArtifactsPanelContext value={{ onOpenArtifact }}>
					<ArtifactToolCard
						toolName="publish_artifact"
						status="completed"
						isError={false}
						artifact={{
							artifactId: "artifact-1",
							version: 2,
							title: "Signup funnel",
							name: "funnel-report.html",
							path: "",
							untrusted: false,
						}}
					/>
				</ArtifactsPanelContext>
			</TooltipProvider>,
		);
		await user.click(screen.getByRole("button", { name: "Open" }));
		expect(onOpenArtifact).toHaveBeenCalledWith("artifact-1", 2);
	});
});

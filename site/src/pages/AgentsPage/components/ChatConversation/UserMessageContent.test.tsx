import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import type {
	MessageDisplayState,
	UserInlineRenderBlock,
} from "./messageHelpers";
import { UserMessageContent } from "./UserMessageContent";

const renderMessage = (
	text: string,
	openFile = vi.fn(),
	blocks: UserInlineRenderBlock[] = [{ type: "response", text }],
) => {
	const workspaceFiles: WorkspaceFilesContextValue = {
		agentId: "agent-1",
		defaultDirectory: "/home/coder",
		repoRoots: ["/home/coder/repo"],
		repositories: new Map(),
		openFile,
	};
	const displayState: MessageDisplayState = {
		shouldHide: false,
		userInlineContent: blocks,
		userFileBlocks: [],
		workspaceFileBlocks: [],
		workspaceFileReferenceCount: 0,
		hasUserMessageBody: true,
		hasFileBlocks: false,
		hasWorkspaceFileReferences: false,
		hasCopyableContent: true,
		needsAssistantBottomSpacer: false,
	};
	render(
		<WorkspaceFilesContext value={workspaceFiles}>
			<UserMessageContent
				displayState={displayState}
				markdown={text}
				isEditing={false}
				onImageClick={vi.fn()}
				onTextFileClick={vi.fn()}
			/>
		</WorkspaceFilesContext>,
	);
	return openFile;
};

describe("UserMessageContent", () => {
	it("opens a referenced file from its chip", async () => {
		const user = userEvent.setup();
		const openFile = renderMessage("explain `/home/coder/repo/main.go` please");
		await user.click(
			screen.getByRole("button", { name: "Open /home/coder/repo/main.go" }),
		);
		expect(openFile).toHaveBeenCalledWith("/home/coder/repo/main.go");
	});

	it("does not open a referenced folder", async () => {
		const user = userEvent.setup();
		const openFile = renderMessage("look in `/home/coder/repo/src/`");
		await user.click(screen.getByTitle("/home/coder/repo/src/"));
		expect(openFile).not.toHaveBeenCalled();
	});

	it("copies chips as their backticked paths", () => {
		const text =
			"compare `/home/coder/repo/main.go` with `/home/coder/repo/src/`";
		renderMessage(text);
		const message = screen.getByText(/compare/);
		window.getSelection()?.selectAllChildren(message);
		const setData = vi.fn();
		fireEvent.copy(message, { clipboardData: { setData } });
		expect(setData).toHaveBeenCalledWith("text/plain", text);
	});

	it("copies skill chips as their aliases", () => {
		renderMessage(" check this", vi.fn(), [
			{
				type: "skill-invocation",
				skill_source: "personal",
				skill_name: "reviewer",
			},
			{ type: "response", text: " check this" },
		]);
		const message = screen.getByText(/check this/);
		window.getSelection()?.selectAllChildren(message);
		const setData = vi.fn();
		fireEvent.copy(message, { clipboardData: { setData } });
		expect(setData).toHaveBeenCalledWith(
			"text/plain",
			"/personal/reviewer check this",
		);
	});

	it("spaces skill chips from the trimmed text around them", () => {
		renderMessage("Review this usingReply in one line.", vi.fn(), [
			{ type: "response", text: "Review this using" },
			{
				type: "skill-invocation",
				skill_source: "personal",
				skill_name: "reviewer",
			},
			{ type: "response", text: "Reply in one line." },
		]);
		const message = screen.getByText(/Review this using/);
		window.getSelection()?.selectAllChildren(message);
		const setData = vi.fn();
		fireEvent.copy(message, { clipboardData: { setData } });
		expect(setData).toHaveBeenCalledWith(
			"text/plain",
			"Review this using /personal/reviewer Reply in one line.",
		);
	});
});

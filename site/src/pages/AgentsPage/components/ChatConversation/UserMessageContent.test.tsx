import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { MessageDisplayState } from "./messageHelpers";
import { UserMessageContent } from "./UserMessageContent";

const baseDisplayState: MessageDisplayState = {
	shouldHide: false,
	userInlineContent: [],
	userFileBlocks: [],
	workspaceFileReferenceCount: 0,
	hasUserMessageBody: false,
	hasFileBlocks: false,
	hasWorkspaceFileReferences: false,
	hasCopyableContent: false,
	needsAssistantBottomSpacer: false,
};

describe("UserMessageContent", () => {
	it("renders a compatibility placeholder for a workspace file reference", () => {
		render(
			<UserMessageContent
				displayState={{
					...baseDisplayState,
					workspaceFileReferenceCount: 1,
					hasWorkspaceFileReferences: true,
				}}
				markdown=""
			/>,
		);

		expect(
			screen.getByText(
				"Workspace file attached. Display support is coming soon.",
			),
		).toBeInTheDocument();
	});

	it("renders a counted compatibility placeholder for workspace file references", () => {
		render(
			<UserMessageContent
				displayState={{
					...baseDisplayState,
					workspaceFileReferenceCount: 2,
					hasWorkspaceFileReferences: true,
				}}
				markdown=""
			/>,
		);

		expect(
			screen.getByText(
				"2 workspace files attached. Display support is coming soon.",
			),
		).toBeInTheDocument();
	});
});

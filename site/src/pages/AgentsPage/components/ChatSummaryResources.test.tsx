import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatContextDirty } from "#/testHelpers/chatEntities";
import { ChatSummaryResources } from "./ChatSummaryResources";

describe("ChatSummaryResources", () => {
	it("keeps empty resource placeholders disabled", () => {
		render(
			<ChatSummaryResources
				usage={{ compressionThreshold: 70, context: { dirty: false } }}
			/>,
		);

		expect(screen.getByRole("button", { name: /^Context/ })).toBeDisabled();
		expect(screen.getByRole("button", { name: /^Skills/ })).toBeDisabled();
		expect(screen.getByRole("button", { name: /^MCP servers/ })).toBeDisabled();
	});

	it("refreshes changed context from the expanded context section", async () => {
		const user = userEvent.setup();
		const onRefreshContext = vi.fn();
		render(
			<ChatSummaryResources
				usage={{ context: MockChatContextDirty }}
				onRefreshContext={onRefreshContext}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /^Context/ }));
		await user.click(screen.getByRole("button", { name: "Refresh context" }));

		expect(onRefreshContext).toHaveBeenCalledTimes(1);
	});
});

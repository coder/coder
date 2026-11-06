import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	MockChatContextClean,
	MockChatContextDirty,
} from "#/testHelpers/chatEntities";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

describe("ContextUsageIndicator", () => {
	it.each([
		{
			name: "refreshes changed context",
			context: MockChatContextDirty,
			refreshes: 1,
		},
		{
			name: "refreshes changed context with a snapshot error",
			context: { ...MockChatContextDirty, error: "one source failed" },
			refreshes: 1,
		},
		{
			name: "does not refresh an unchanged snapshot error",
			context: { ...MockChatContextClean, error: "one source failed" },
			refreshes: 0,
		},
	])("$name", async ({ context, refreshes }) => {
		const user = userEvent.setup();
		const onRefreshContext = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{
					usedTokens: 12_000,
					contextLimitTokens: 200_000,
					context,
				}}
				onRefreshContext={onRefreshContext}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		const refreshButton = screen.queryByRole("button", {
			name: "Refresh context",
		});
		if (refreshButton) {
			await user.click(refreshButton);
		}
		await user.keyboard("{Escape}");
		expect(onRefreshContext).toHaveBeenCalledTimes(refreshes);
	});
});

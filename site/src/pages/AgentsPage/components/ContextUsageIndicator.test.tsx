import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockChatContextDirty } from "#/testHelpers/chatEntities";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

describe("ContextUsageIndicator", () => {
	it.each([
		{
			name: "refreshes changed context",
			context: MockChatContextDirty,
		},
		{
			name: "refreshes changed context with a snapshot error",
			context: { ...MockChatContextDirty, error: "one source failed" },
		},
	])("$name", async ({ context }) => {
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
		await user.click(screen.getByRole("button", { name: "Refresh context" }));
		expect(onRefreshContext).toHaveBeenCalledTimes(1);
	});
});

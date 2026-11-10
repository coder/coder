import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	MockChatContextDirty,
	MockChatContextResourceFailures,
} from "#/testHelpers/chatEntities";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

describe("ContextUsageIndicator", () => {
	it("opens server management from an MCP server failure", async () => {
		const user = userEvent.setup();
		const onManageServers = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{ context: MockChatContextResourceFailures }}
				onManageServers={onManageServers}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		await user.click(screen.getByRole("button", { name: /MCP Servers/ }));
		await user.click(screen.getByRole("button", { name: "Manage servers" }));
		expect(onManageServers).toHaveBeenCalledTimes(1);
	});

	it("refreshes context after opening the popover with a click", async () => {
		const user = userEvent.setup();
		const onRefreshContext = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{
					usedTokens: 12_000,
					contextLimitTokens: 200_000,
					context: MockChatContextDirty,
				}}
				onRefreshContext={onRefreshContext}
			/>,
		);
		await user.click(screen.getByRole("button", { name: /Context usage/ }));
		await user.click(screen.getByRole("button", { name: "Refresh context" }));
		expect(onRefreshContext).toHaveBeenCalledTimes(1);
	});

	it("closes the popover and passes the trigger when opening Details from the keyboard", async () => {
		const user = userEvent.setup();
		const onOpenDetails = vi.fn();
		render(
			<ContextUsageIndicator
				usage={{ usedTokens: 12_000, contextLimitTokens: 200_000 }}
				onOpenDetails={onOpenDetails}
			/>,
		);
		const trigger = screen.getByRole("button", { name: /Context usage/ });
		await user.tab();
		expect(trigger).toHaveFocus();
		await user.keyboard("{Enter}");
		const details = await screen.findByRole("button", { name: "Details" });
		details.focus();
		await user.keyboard("{Enter}");
		expect(onOpenDetails).toHaveBeenCalledExactlyOnceWith(trigger);
		expect(
			screen.queryByRole("button", { name: "Details" }),
		).not.toBeInTheDocument();
		expect(trigger).not.toHaveFocus();
	});
});

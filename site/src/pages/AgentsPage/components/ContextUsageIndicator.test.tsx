import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

it("offers a context refresh when an MCP server failed to connect", async () => {
	const onRefreshContext = vi.fn();
	render(
		<ContextUsageIndicator
			usage={{
				context: {
					dirty: false,
					resources: [
						{
							source: "storybook",
							kind: "mcp_server",
							size_bytes: 0,
							status: "unreadable",
						},
					],
				},
			}}
			onRefreshContext={onRefreshContext}
		/>,
	);

	await userEvent.hover(screen.getByRole("button"));
	await userEvent.click(
		await screen.findByRole("button", { name: "Refresh context" }),
	);

	expect(onRefreshContext).toHaveBeenCalledOnce();
});

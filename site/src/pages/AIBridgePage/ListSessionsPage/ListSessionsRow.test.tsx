import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Table, TableBody } from "#/components/Table/Table";
import { MockSession } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ListSessionsRow } from "./ListSessionsRow";

const mockMultiProviderSession = {
	...MockSession,
	providers: ["anthropic", "openai", "copilot"],
};

const renderMultiProviderRow = () => {
	const onClick = vi.fn();
	renderComponent(
		<Table>
			<TableBody>
				<ListSessionsRow session={mockMultiProviderSession} onClick={onClick} />
			</TableBody>
		</Table>,
	);
	return onClick;
};

const getCountBadge = () => screen.getByRole("button", { name: "3 providers" });

describe("count badge inside the clickable session row", () => {
	it("pins the list on click, and neither the badge nor the list opens the row", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.click(getCountBadge());
		await user.unhover(getCountBadge());
		await user.click(await screen.findByRole("dialog"));
		expect(getCountBadge()).toHaveAttribute("aria-expanded", "true");
		expect(onClick).not.toHaveBeenCalled();
	});
});

describe("network requests badge inside the clickable session row", () => {
	it("opens the row when the badge named by its counts is clicked", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.click(
			screen.getByRole("button", { name: "Network requests: 23, blocked: 2" }),
		);
		expect(onClick).toHaveBeenCalledTimes(1);
	});
});

describe("last prompt tooltip inside the clickable session row", () => {
	it("keeps clicks inside the tooltip from opening the row", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.hover(screen.getByText(MockSession.last_prompt ?? ""));
		await user.click(await screen.findByRole("tooltip"));
		expect(onClick).not.toHaveBeenCalled();
	});
});

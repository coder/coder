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
	it("opens the row when the badge is clicked while its list is shown", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.hover(getCountBadge());
		await screen.findByRole("tooltip");
		await user.click(getCountBadge());
		expect(onClick).toHaveBeenCalledTimes(1);
	});

	it("opens the row when the focused badge is activated with Enter", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.tab();
		await screen.findByRole("tooltip");
		await user.keyboard("{Enter}");
		expect(onClick).toHaveBeenCalledTimes(1);
	});

	it("keeps clicks inside the list from opening the row", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.hover(getCountBadge());
		await user.click(await screen.findByRole("tooltip"));
		expect(onClick).not.toHaveBeenCalled();
	});

	it("opens the row on a touch tap because touch cannot open the list", async () => {
		const user = userEvent.setup();
		const onClick = renderMultiProviderRow();

		await user.pointer({ keys: "[TouchA]", target: getCountBadge() });
		expect(onClick).toHaveBeenCalledTimes(1);
	});
});

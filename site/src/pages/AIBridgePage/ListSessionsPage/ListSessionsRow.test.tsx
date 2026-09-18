import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { Table, TableBody } from "#/components/Table/Table";
import { MockSession } from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ListSessionsRow } from "./ListSessionsRow";

it("keeps count badge interaction separate from row navigation", async () => {
	const user = userEvent.setup();
	const onClick = vi.fn();
	renderComponent(
		<Table>
			<TableBody>
				<ListSessionsRow
					session={{
						...MockSession,
						providers: ["anthropic", "openai", "copilot"],
					}}
					onClick={onClick}
				/>
			</TableBody>
		</Table>,
	);

	const badge = screen.getByRole("button", { name: "3 providers" });
	await user.click(badge);
	await user.keyboard("{Enter}");
	expect(onClick).not.toHaveBeenCalled();

	await user.click(screen.getByRole("row"));
	expect(onClick).toHaveBeenCalledTimes(1);
});

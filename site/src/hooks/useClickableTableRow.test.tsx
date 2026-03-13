import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, MouseEventHandler } from "react";
import { useClickableTableRow } from "./useClickableTableRow";

type RowFixtureProps = {
	onClick: MouseEventHandler<HTMLTableRowElement>;
};

const RowFixture: FC<RowFixtureProps> = ({ onClick }) => {
	const rowProps = useClickableTableRow({ onClick });

	return (
		<table>
			<tbody>
				<tr data-testid="clickable-row" {...rowProps}>
					<td>Task row</td>
				</tr>
			</tbody>
		</table>
	);
};

describe(useClickableTableRow.name, () => {
	it("does not force button semantics onto table rows", () => {
		render(<RowFixture onClick={vi.fn()} />);

		const row = screen.getByTestId("clickable-row");
		expect(row).not.toHaveAttribute("role");
		expect(row).not.toHaveAttribute("tabindex");
	});

	it("keeps click-based navigation behavior", async () => {
		const onClick = vi.fn();
		const user = userEvent.setup();
		render(<RowFixture onClick={onClick} />);

		await user.click(screen.getByTestId("clickable-row"));
		expect(onClick).toHaveBeenCalledTimes(1);
	});
});

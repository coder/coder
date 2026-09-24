import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, ReactNode } from "react";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { MockWorkspace } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const ClickableRow: FC<{ onClick: () => void; children: ReactNode }> = ({
	onClick,
	children,
}) => {
	const { hover, ...rowProps } = useClickableTableRow({ onClick });
	return (
		<table>
			<tbody>
				<tr {...rowProps}>
					<td>{children}</td>
				</tr>
			</tbody>
		</table>
	);
};

const renderInClickableRow = () => {
	const onConfirm = vi.fn();
	const onCancel = vi.fn();
	const onRowClick = vi.fn();

	render(
		<ClickableRow onClick={onRowClick}>
			<WorkspaceDeleteDialog
				workspace={MockWorkspace}
				canDeleteFailedWorkspace={false}
				isOpen
				onConfirm={onConfirm}
				onCancel={onCancel}
			/>
		</ClickableRow>,
	);

	return { onConfirm, onCancel, onRowClick };
};

describe("WorkspaceDeleteDialog", () => {
	it("does not confirm or activate the parent row when typing a wrong name with spaces and pressing Enter", async () => {
		const user = userEvent.setup();
		const { onConfirm, onCancel, onRowClick } = renderInClickableRow();

		const input = screen.getByLabelText("Workspace name");
		await user.type(input, "wrong name{Enter}", { skipClick: true });

		expect(onConfirm).not.toHaveBeenCalled();
		expect(onCancel).not.toHaveBeenCalled();
		expect(onRowClick).not.toHaveBeenCalled();
		expect(input).toHaveValue("wrong name");
		expect(input).toHaveFocus();
	});

	it("confirms on Enter without activating the parent row when the name matches", async () => {
		const user = userEvent.setup();
		const { onConfirm, onRowClick } = renderInClickableRow();

		await user.type(
			screen.getByLabelText("Workspace name"),
			`${MockWorkspace.name}{Enter}`,
			{ skipClick: true },
		);

		expect(onConfirm).toHaveBeenCalledWith(false);
		expect(onRowClick).not.toHaveBeenCalled();
	});
});

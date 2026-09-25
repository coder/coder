import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, ReactNode } from "react";
import { createPortal } from "react-dom";
import { useClickableTableRow } from "./useClickableTableRow";

const Row: FC<{
	onClick: () => void;
	onMiddleClick: () => void;
	children: ReactNode;
}> = ({ onClick, onMiddleClick, children }) => {
	const { hover, ...rowProps } = useClickableTableRow({
		onClick,
		onMiddleClick,
	});
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

describe(useClickableTableRow.name, () => {
	it("opens on middle-click in the row but ignores middle-clicks from portaled content", async () => {
		const user = userEvent.setup();
		const onMiddleClick = vi.fn();
		render(
			<Row onClick={vi.fn()} onMiddleClick={onMiddleClick}>
				<span>Row cell</span>
				{createPortal(<input aria-label="Dialog input" />, document.body)}
			</Row>,
		);

		await user.pointer({
			keys: "[MouseMiddle]",
			target: screen.getByLabelText("Dialog input"),
		});
		expect(onMiddleClick).not.toHaveBeenCalled();

		await user.pointer({
			keys: "[MouseMiddle]",
			target: screen.getByText("Row cell"),
		});
		expect(onMiddleClick).toHaveBeenCalledTimes(1);
	});
});

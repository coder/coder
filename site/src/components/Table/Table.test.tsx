import { render, screen } from "@testing-library/react";
import { TableHead } from "./Table";

const renderTableHead = (props?: React.ComponentProps<typeof TableHead>) => {
	render(
		<table>
			<thead>
				<tr>
					<TableHead {...props}>Name</TableHead>
				</tr>
			</thead>
		</table>,
	);

	const tableHead = screen.getByText("Name").closest("th");
	if (!tableHead) {
		throw new Error("Expected TableHead to render a th element.");
	}

	return tableHead;
};

describe("TableHead", () => {
	it("renders with scope col by default", () => {
		const tableHead = renderTableHead();

		expect(tableHead).toHaveAttribute("scope", "col");
	});

	it("allows overriding scope", () => {
		const tableHead = renderTableHead({ scope: "row" });

		expect(tableHead).toHaveAttribute("scope", "row");
	});
});

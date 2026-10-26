import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { SearchField } from "./SearchField";

const ControlledSearchField: React.FC = () => {
	const [value, setValue] = useState("");
	return (
		<TooltipProvider>
			<SearchField value={value} onChange={setValue} aria-label="Search" />
		</TooltipProvider>
	);
};

describe("SearchField", () => {
	it("keeps focus in the field after Clear search", async () => {
		const user = userEvent.setup();
		render(<ControlledSearchField />);
		const input = screen.getByRole("textbox", { name: "Search" });

		await user.type(input, "probe");
		await user.click(screen.getByRole("button", { name: "Clear search" }));

		expect(input).toHaveValue("");
		expect(input).toHaveFocus();
	});
});

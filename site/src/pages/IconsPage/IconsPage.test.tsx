import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import IconsPage from "./IconsPage";

describe("IconsPage", () => {
	it("resets the search from the empty state", async () => {
		const user = userEvent.setup();
		render(<IconsPage />);

		const filter = screen.getByRole("textbox", { name: "Filter" });
		await user.type(filter, "no-matching-icon");
		await user.click(screen.getByRole("button", { name: "Reset filters" }));

		expect(filter).toHaveValue("");
	});
});

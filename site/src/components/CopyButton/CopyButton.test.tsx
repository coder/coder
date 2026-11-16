import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { CopyButton } from "./CopyButton";

describe("CopyButton", () => {
	it("announces a successful copy", async () => {
		const user = userEvent.setup();
		renderComponent(
			<CopyButton text="crimson-cobra-7" label="Copy workspace name" />,
		);

		const status = screen.getByRole("status");
		expect(status).toBeEmptyDOMElement();
		await user.click(
			screen.getByRole("button", { name: "Copy workspace name" }),
		);

		expect(status).toHaveTextContent("Copied");
	});
});

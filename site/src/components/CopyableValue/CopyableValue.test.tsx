import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { CopyableValue } from "./CopyableValue";

describe("CopyableValue", () => {
	it("announces a successful copy", async () => {
		const user = userEvent.setup();
		renderComponent(
			<CopyableValue value="crimson-cobra-7">crimson-cobra-7</CopyableValue>,
		);

		const status = screen.getByRole("status");
		expect(status).toBeEmptyDOMElement();
		await user.click(screen.getByRole("button", { name: "crimson-cobra-7" }));

		expect(status).toHaveTextContent("Copied");
	});
});

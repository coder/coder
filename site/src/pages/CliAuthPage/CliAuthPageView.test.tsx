import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { CliAuthPageView } from "./CliAuthPageView";

describe("CliAuthPageView", () => {
	it("announces a copied session token", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		render(<CliAuthPageView sessionToken="hunter2" />);

		await user.click(
			screen.getByRole("button", { name: "Copy session token" }),
		);

		expect(writeText).toHaveBeenCalledWith("hunter2");
		expect(screen.getByRole("status")).toHaveTextContent("Copied");
	});
});

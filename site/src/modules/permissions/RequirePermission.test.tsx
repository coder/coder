import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { RequirePermission } from "./RequirePermission";

describe("RequirePermission", () => {
	it("focuses the return link on denial without stealing focus on rerender", async () => {
		const user = userEvent.setup();
		const deniedPage = () => (
			<>
				<button type="button">Other action</button>
				<RequirePermission isFeatureVisible={false} />
			</>
		);
		const { rerender } = renderComponent(deniedPage());
		const returnLink = screen.getByRole("link", { name: "Go to workspaces" });

		expect(returnLink).toHaveFocus();

		await user.click(screen.getByRole("button", { name: "Other action" }));
		rerender(deniedPage());

		expect(screen.getByRole("button", { name: "Other action" })).toHaveFocus();
	});
});

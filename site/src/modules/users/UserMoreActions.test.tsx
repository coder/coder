import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { UserMoreActions } from "./UserMoreActions";

describe("UserMoreActions", () => {
	it("names the menu after the user it acts on", async () => {
		const onAction = vi.fn();
		render(
			<UserMoreActions
				user={MockUserMember}
				me={MockUserOwner.id}
				onAction={onAction}
			/>,
		);

		const user = userEvent.setup();
		await user.click(
			screen.getByRole("button", {
				name: `Open menu for ${MockUserMember.username}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Suspend…" }));
		expect(onAction).toHaveBeenCalledWith({
			type: "suspend",
			user: MockUserMember,
		});
	});
});

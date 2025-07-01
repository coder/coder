import { screen } from "@testing-library/react";
import { Popover } from "components/deprecated/Popover/Popover";
import { MockUserOwner } from "testHelpers/entities";
import { render, waitForLoaderToBeRemoved } from "testHelpers/renderHelpers";
import { Language, UserDropdownContent } from "./UserDropdownContent";

describe("UserDropdownContent", () => {
	it("has the correct link for the account item", async () => {
		render(
			<Popover>
				<UserDropdownContent user={MockUserOwner} onSignOut={jest.fn()} />
			</Popover>,
		);
		await waitForLoaderToBeRemoved();

		const link = screen.getByText(Language.accountLabel).closest("a");
		if (!link) {
			throw new Error("Anchor tag not found for the account menu item");
		}

		expect(link.getAttribute("href")).toBe("/settings/account");
	});

	it("calls the onSignOut function", async () => {
		const onSignOut = jest.fn();
		render(
			<Popover>
				<UserDropdownContent user={MockUserOwner} onSignOut={onSignOut} />
			</Popover>,
		);
		await waitForLoaderToBeRemoved();
		screen.getByText(Language.signOutLabel).click();
		expect(onSignOut).toBeCalledTimes(1);
	});
});

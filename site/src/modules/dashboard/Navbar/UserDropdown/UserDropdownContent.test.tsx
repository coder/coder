import { MockUserOwner } from "testHelpers/entities";
import { render, waitForLoaderToBeRemoved } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Language, UserDropdownContent } from "./UserDropdownContent";

const renderUserDropdownContent = (props: { onSignOut: () => void }) => {
	return render(
		<DropdownMenu defaultOpen>
			<DropdownMenuTrigger>Open</DropdownMenuTrigger>
			<DropdownMenuContent>
				<UserDropdownContent
					user={MockUserOwner}
					onSignOut={props.onSignOut}
					supportLinks={[]}
				/>
			</DropdownMenuContent>
		</DropdownMenu>,
	);
};

describe("UserDropdownContent", () => {
	it("has the correct link for the account item", async () => {
		renderUserDropdownContent({ onSignOut: vi.fn() });
		await waitForLoaderToBeRemoved();

		const link = screen.getByText(Language.accountLabel).closest("a");
		if (!link) {
			throw new Error("Anchor tag not found for the account menu item");
		}

		expect(link.getAttribute("href")).toBe("/settings/account");
	});

	it("calls the onSignOut function", async () => {
		const onSignOut = vi.fn();
		renderUserDropdownContent({ onSignOut });
		await waitForLoaderToBeRemoved();
		screen.getByText(Language.signOutLabel).click();
		expect(onSignOut).toBeCalledTimes(1);
	});
});

import { screen } from "@testing-library/react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { MockUserOwner } from "#/testHelpers/entities";
import { render, waitForLoaderToBeRemoved } from "#/testHelpers/renderHelpers";
import { UserDropdownContent } from "./UserDropdownContent";

const renderUserDropdownContent = (props: {
	canViewOrganizations?: boolean;
	onSignOut: () => void;
}) => {
	return render(
		<DropdownMenu defaultOpen>
			<DropdownMenuTrigger>Open</DropdownMenuTrigger>
			<DropdownMenuContent>
				<UserDropdownContent
					user={MockUserOwner}
					canViewOrganizations={props.canViewOrganizations}
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

		const link = screen.getByText("Account").closest("a");
		if (!link) {
			throw new Error("Anchor tag not found for the account menu item");
		}

		expect(link.getAttribute("href")).toBe("/settings/account");
	});

	it("calls the onSignOut function", async () => {
		const onSignOut = vi.fn();
		renderUserDropdownContent({ onSignOut });
		await waitForLoaderToBeRemoved();
		screen.getByText("Sign Out").click();
		expect(onSignOut).toBeCalledTimes(1);
	});

	it("can show the organizations item", async () => {
		renderUserDropdownContent({
			canViewOrganizations: true,
			onSignOut: vi.fn(),
		});
		await waitForLoaderToBeRemoved();

		const link = screen.getByText("Organizations").closest("a");
		if (!link) {
			throw new Error("Anchor tag not found for the organizations menu item");
		}

		expect(link.getAttribute("href")).toBe("/organizations");
	});
});

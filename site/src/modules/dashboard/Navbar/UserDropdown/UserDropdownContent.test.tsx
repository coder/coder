import { screen } from "@testing-library/react";
import type { ReactNode } from "react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { MockUserOwner } from "#/testHelpers/entities";
import { render, waitForLoaderToBeRemoved } from "#/testHelpers/renderHelpers";
import { UserDropdownContent } from "./UserDropdownContent";

const renderUserDropdownContent = (props: {
	onSignOut: () => void;
	profileExtra?: ReactNode;
}) => {
	return render(
		<DropdownMenu defaultOpen>
			<DropdownMenuTrigger>Open</DropdownMenuTrigger>
			<DropdownMenuContent>
				<UserDropdownContent
					user={MockUserOwner}
					onSignOut={props.onSignOut}
					profileExtra={props.profileExtra}
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

	it("renders the profile extra content when provided", async () => {
		renderUserDropdownContent({
			onSignOut: vi.fn(),
			profileExtra: <div>AI spend - $819 / $1,200 USD</div>,
		});
		await waitForLoaderToBeRemoved();

		expect(
			screen.getByText("AI spend - $819 / $1,200 USD"),
		).toBeInTheDocument();
	});
});

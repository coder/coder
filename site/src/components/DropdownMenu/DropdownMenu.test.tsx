import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "./DropdownMenu";

const renderMenu = (onOpenChange = vi.fn()) => {
	render(
		<>
			<button type="button">Page action</button>
			<DropdownMenu onOpenChange={onOpenChange}>
				<DropdownMenuTrigger>Actions</DropdownMenuTrigger>
				<DropdownMenuContent>
					<DropdownMenuItem>Settings</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</>,
	);
	return onOpenChange;
};

describe("DropdownMenu", () => {
	it("keeps the rest of the page exposed to assistive technology while open", async () => {
		const user = userEvent.setup();
		renderMenu();

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });

		// Role queries skip elements hidden with aria-hidden.
		screen.getByRole("button", { name: "Page action" });
	});

	it("closes when focus moves outside the menu", async () => {
		const user = userEvent.setup();
		const onOpenChange = renderMenu();

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });
		act(() => screen.getByRole("button", { name: "Page action" }).focus());

		expect(onOpenChange).toHaveBeenLastCalledWith(false);
	});
});

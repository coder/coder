import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "./DropdownMenu";

const renderMenu = () => {
	const onOpenChange = vi.fn();
	const onPageAction = vi.fn();
	render(
		<>
			<button type="button" onClick={onPageAction}>
				Page action
			</button>
			<DropdownMenu onOpenChange={onOpenChange}>
				<DropdownMenuTrigger>Actions</DropdownMenuTrigger>
				<DropdownMenuContent>
					<DropdownMenuItem>Settings</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</>,
	);
	return { onOpenChange, onPageAction };
};

describe("DropdownMenu", () => {
	it("keeps the rest of the page exposed to assistive technology while open", async () => {
		const user = userEvent.setup();
		renderMenu();

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });

		// Role queries skip elements hidden with aria-hidden.
		expect(
			screen.queryByRole("button", { name: "Page action" }),
		).not.toBeNull();
	});

	it("closes when focus moves outside the menu", async () => {
		const user = userEvent.setup();
		const { onOpenChange } = renderMenu();

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });
		act(() => screen.getByRole("button", { name: "Page action" }).focus());

		expect(onOpenChange).toHaveBeenLastCalledWith(false);
	});

	it("closes without activating the element clicked outside", async () => {
		const user = userEvent.setup();
		const { onOpenChange, onPageAction } = renderMenu();

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });
		await user.click(screen.getByRole("button", { name: "Page action" }));

		expect(onOpenChange).toHaveBeenLastCalledWith(false);
		expect(onPageAction).not.toHaveBeenCalled();

		await user.click(screen.getByRole("button", { name: "Page action" }));
		expect(onPageAction).toHaveBeenCalledTimes(1);
	});

	it("does not swallow a later click after a right click dismisses it", async () => {
		const user = userEvent.setup();
		const { onOpenChange, onPageAction } = renderMenu();
		const pageAction = screen.getByRole("button", { name: "Page action" });

		await user.click(screen.getByRole("button", { name: "Actions" }));
		await screen.findByRole("menuitem", { name: "Settings" });
		await user.pointer({ keys: "[MouseRight]", target: pageAction });
		expect(onOpenChange).toHaveBeenLastCalledWith(false);

		await user.click(pageAction);
		expect(onPageAction).toHaveBeenCalledTimes(1);
	});
});

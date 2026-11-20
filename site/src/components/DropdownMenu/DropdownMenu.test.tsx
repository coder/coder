import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "./DropdownMenu";

const renderMenu = ({ modal }: { modal?: boolean } = {}) => {
	const onOpenChange = vi.fn();
	render(
		<>
			<button type="button">Page action</button>
			<DropdownMenu modal={modal} onOpenChange={onOpenChange}>
				<DropdownMenuTrigger>Actions</DropdownMenuTrigger>
				<DropdownMenuContent>
					<DropdownMenuItem>Settings</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</>,
	);
	return { onOpenChange };
};

const openMenu = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Actions" }));
	await screen.findByRole("menuitem", { name: "Settings" });
};

const clickCatcherId = "dropdown-menu-click-catcher";

describe("DropdownMenu", () => {
	it("keeps the rest of the page exposed to assistive technology while open", async () => {
		const user = userEvent.setup();
		renderMenu();

		await openMenu(user);

		// Role queries skip elements hidden with aria-hidden.
		expect(
			screen.queryByRole("button", { name: "Page action" }),
		).not.toBeNull();
	});

	it("closes when focus moves outside the menu", async () => {
		const user = userEvent.setup();
		const { onOpenChange } = renderMenu();

		await openMenu(user);
		act(() => screen.getByRole("button", { name: "Page action" }).focus());

		expect(onOpenChange).toHaveBeenLastCalledWith(false);
	});

	it("blocks the page until the press that closes it ends", async () => {
		const user = userEvent.setup();
		const { onOpenChange } = renderMenu();

		await openMenu(user);
		const catcher = screen.getByTestId(clickCatcherId);

		await user.pointer({ keys: "[MouseLeft>]", target: catcher });
		expect(onOpenChange).toHaveBeenLastCalledWith(false);
		expect(screen.getByTestId(clickCatcherId)).toBe(catcher);

		await user.pointer({ keys: "[/MouseLeft]", target: catcher });
		expect(screen.queryByTestId(clickCatcherId)).toBeNull();
	});

	it("returns focus to the trigger after an outside press closes it", async () => {
		const user = userEvent.setup();
		renderMenu();

		await openMenu(user);
		await user.click(screen.getByTestId(clickCatcherId));

		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Actions" })).toHaveFocus(),
		);
	});

	it("unblocks the page when a right click closes it", async () => {
		const user = userEvent.setup();
		const { onOpenChange } = renderMenu();

		await openMenu(user);
		await user.pointer({
			keys: "[MouseRight]",
			target: screen.getByTestId(clickCatcherId),
		});

		expect(onOpenChange).toHaveBeenLastCalledWith(false);
		expect(screen.queryByTestId(clickCatcherId)).toBeNull();
	});

	it("does not block the page for a modal menu", async () => {
		const user = userEvent.setup();
		renderMenu({ modal: true });

		await openMenu(user);
		expect(screen.queryByTestId(clickCatcherId)).toBeNull();
	});
});

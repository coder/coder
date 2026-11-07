import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Button } from "#/components/Button/Button";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "#/components/ContextMenu/ContextMenu";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "./DropdownMenu";

it("activates an item with the keyboard and returns focus", async () => {
	const user = userEvent.setup();
	const onClick = vi.fn();
	const onOpenChange = vi.fn();
	render(
		<DropdownMenu onOpenChange={onOpenChange}>
			<DropdownMenuTrigger render={<Button />}>Actions</DropdownMenuTrigger>
			<DropdownMenuContent>
				<DropdownMenuItem onClick={onClick}>Run</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>,
	);
	screen.getByRole("button", { name: "Actions" }).focus();
	await user.keyboard("{ArrowDown}");
	await waitFor(() =>
		expect(screen.getByRole("menuitem", { name: "Run" })).toHaveFocus(),
	);
	await user.keyboard("{Enter}");
	expect(onClick).toHaveBeenCalledOnce();
	expect(onOpenChange).toHaveBeenLastCalledWith(false, expect.anything());
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Actions" })).toHaveFocus(),
	);
});

it("keeps a setting menu open for repeated toggles", async () => {
	const user = userEvent.setup();
	const onCheckedChange = vi.fn();
	const onOpenChange = vi.fn();
	render(
		<DropdownMenu defaultOpen onOpenChange={onOpenChange}>
			<DropdownMenuContent>
				<DropdownMenuCheckboxItem
					closeOnClick={false}
					onCheckedChange={onCheckedChange}
				>
					Sound
				</DropdownMenuCheckboxItem>
			</DropdownMenuContent>
		</DropdownMenu>,
	);
	await user.click(screen.getByRole("menuitemcheckbox", { name: "Sound" }));
	await user.click(screen.getByRole("menuitemcheckbox", { name: "Sound" }));
	expect(onCheckedChange).toHaveBeenNthCalledWith(1, true, expect.anything());
	expect(onCheckedChange).toHaveBeenNthCalledWith(2, false, expect.anything());
	expect(onOpenChange).not.toHaveBeenCalled();
});

it("activates a nested submenu action", async () => {
	const user = userEvent.setup();
	const onClick = vi.fn();
	const onOpenChange = vi.fn();
	render(
		<DropdownMenu defaultOpen onOpenChange={onOpenChange}>
			<DropdownMenuContent>
				<DropdownMenuSub>
					<DropdownMenuSubTrigger>More</DropdownMenuSubTrigger>
					<DropdownMenuSubContent>
						<DropdownMenuItem onClick={onClick}>Export</DropdownMenuItem>
					</DropdownMenuSubContent>
				</DropdownMenuSub>
			</DropdownMenuContent>
		</DropdownMenu>,
	);
	screen.getByRole("menuitem", { name: "More" }).focus();
	await user.keyboard("{ArrowRight}");
	await waitFor(() =>
		expect(screen.getByRole("menuitem", { name: "Export" })).toHaveFocus(),
	);
	await user.keyboard("{Enter}");
	expect(onClick).toHaveBeenCalledOnce();
	expect(onOpenChange).toHaveBeenLastCalledWith(false, expect.anything());
});

it("honors a custom return-focus callback", async () => {
	const user = userEvent.setup();
	const onCloseAutoFocus = vi.fn((event: Event) => {
		event.preventDefault();
		screen.getByRole("button", { name: "Return" }).focus();
	});
	render(
		<>
			<button type="button">Return</button>
			<DropdownMenu defaultOpen>
				<DropdownMenuContent onCloseAutoFocus={onCloseAutoFocus}>
					<DropdownMenuItem>Run</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>
		</>,
	);
	await user.click(screen.getByRole("menuitem", { name: "Run" }));
	await waitFor(() => expect(onCloseAutoFocus).toHaveBeenCalledOnce());
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Return" })).toHaveFocus(),
	);
});

it("opens a context menu with a right click and activates an action", async () => {
	const user = userEvent.setup();
	const onClick = vi.fn();
	render(
		<ContextMenu>
			<ContextMenuTrigger>Workspace</ContextMenuTrigger>
			<ContextMenuContent>
				<ContextMenuItem onClick={onClick}>Copy</ContextMenuItem>
			</ContextMenuContent>
		</ContextMenu>,
	);
	await user.pointer({
		target: screen.getByText("Workspace"),
		keys: "[MouseRight]",
	});
	await user.click(await screen.findByRole("menuitem", { name: "Copy" }));
	expect(onClick).toHaveBeenCalledOnce();
});

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { describe, expect, it, vi } from "vitest";
import { Button } from "#/components/Button/Button";
import {
	Drawer,
	DrawerClose,
	DrawerContent,
	DrawerTitle,
	DrawerTrigger,
} from "#/components/Drawer/Drawer";
import {
	Dialog,
	DialogClose,
	DialogContent,
	DialogTitle,
	DialogTrigger,
} from "./Dialog";

describe.each([
	{
		name: "Dialog",
		Root: Dialog,
		Content: DialogContent,
		Title: DialogTitle,
		Trigger: DialogTrigger,
		Close: DialogClose,
	},
	{
		name: "Drawer",
		Root: Drawer,
		Content: DrawerContent,
		Title: DrawerTitle,
		Trigger: DrawerTrigger,
		Close: DrawerClose,
	},
])("$name", ({ Root, Content, Title, Trigger, Close }) => {
	it("closes with Escape and restores focus to its trigger", async () => {
		const user = userEvent.setup();
		const onOpenChange = vi.fn();
		render(
			<Root onOpenChange={onOpenChange}>
				<Trigger render={<Button />}>Open</Trigger>
				<Content>
					<Title>Settings</Title>
					<Close>Close</Close>
				</Content>
			</Root>,
		);
		await user.click(screen.getByRole("button", { name: "Open" }));
		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenLastCalledWith(false, expect.anything());
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Open" })).toHaveFocus(),
		);
	});

	it("closes through a composed close button", async () => {
		const user = userEvent.setup();
		const onOpenChange = vi.fn();
		render(
			<Root defaultOpen onOpenChange={onOpenChange}>
				<Content>
					<Title>Settings</Title>
					<Close render={<Button />}>Done</Close>
				</Content>
			</Root>,
		);
		await user.click(screen.getByRole("button", { name: "Done" }));
		expect(onOpenChange).toHaveBeenCalledWith(false, expect.anything());
	});

	it("allows a dismissal guard to cancel Escape", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();
		render(
			<Root defaultOpen onOpenChange={(_, details) => details.cancel()}>
				<Content>
					<Title>Consent</Title>
					<Close onClick={onClose}>Confirm</Close>
				</Content>
			</Root>,
		);
		await user.keyboard("{Escape}");
		await user.click(screen.getByRole("button", { name: "Confirm" }));
		expect(onClose).toHaveBeenCalledOnce();
	});

	it("honors custom open and close focus callbacks", async () => {
		const user = userEvent.setup();
		const onInputFocus = vi.fn();
		function Example() {
			const input = useRef<HTMLInputElement>(null);
			const returnButton = useRef<HTMLButtonElement>(null);
			return (
				<>
					<Button ref={returnButton}>Return here</Button>
					<Root>
						<Trigger>Open</Trigger>
						<Content
							onOpenAutoFocus={(event) => {
								event.preventDefault();
								input.current?.focus();
							}}
							onCloseAutoFocus={(event) => {
								event.preventDefault();
								returnButton.current?.focus();
							}}
						>
							<Title>Settings</Title>
							<Close>Close</Close>
							<input aria-label="Name" ref={input} onFocus={onInputFocus} />
						</Content>
					</Root>
				</>
			);
		}
		render(<Example />);
		await user.click(screen.getByRole("button", { name: "Open" }));
		await waitFor(() => expect(onInputFocus).toHaveBeenCalled());
		await user.keyboard("{Escape}");
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Return here" })).toHaveFocus(),
		);
	});
});

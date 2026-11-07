import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "./Select";

it("submits the selected value after keyboard selection", async () => {
	const user = userEvent.setup();
	const onSubmit = vi.fn();
	render(
		<form
			onSubmit={(event) => {
				event.preventDefault();
				onSubmit(new FormData(event.currentTarget).get("fruit"));
			}}
		>
			<Select name="fruit" defaultValue="apple">
				<SelectTrigger aria-label="Fruit">
					<SelectValue />
				</SelectTrigger>
				<SelectContent>
					<SelectGroup>
						<SelectLabel>Fruits</SelectLabel>
						<SelectItem value="apple">Apple</SelectItem>
						<SelectItem value="banana">Banana</SelectItem>
					</SelectGroup>
				</SelectContent>
			</Select>
			<button type="submit">Save</button>
		</form>,
	);
	screen.getByRole("combobox", { name: "Fruit" }).focus();
	await user.keyboard("{ArrowDown}");
	await screen.findByRole("option", { name: "Apple" });
	await user.keyboard("{ArrowDown}{Enter}");
	await user.click(screen.getByRole("button", { name: "Save" }));
	expect(onSubmit).toHaveBeenCalledWith("banana");
});

it("selects from an initially empty controlled value", async () => {
	const user = userEvent.setup();
	const onValueChange = vi.fn();
	render(
		<Select value="" onValueChange={onValueChange}>
			<SelectTrigger aria-label="Fruit">
				<SelectValue placeholder="Choose fruit" />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value="apple">Apple</SelectItem>
			</SelectContent>
		</Select>,
	);
	await user.click(screen.getByRole("combobox", { name: "Fruit" }));
	await user.click(await screen.findByRole("option", { name: "Apple" }));
	await waitFor(() => expect(onValueChange).toHaveBeenCalledWith("apple"));
});

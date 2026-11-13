import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Dialog, DialogContent, DialogTitle } from "#/components/Dialog/Dialog";
import { MultiSelectCombobox } from "./MultiSelectCombobox";

const options = [
	{ value: "docker", label: "Docker" },
	{ value: "kubernetes", label: "Kubernetes" },
];

describe("MultiSelectCombobox", () => {
	it("closes only its list on Escape inside a dialog", async () => {
		const user = userEvent.setup();
		const onOpenChange = vi.fn();
		render(
			<Dialog open onOpenChange={onOpenChange}>
				<DialogContent aria-describedby={undefined}>
					<DialogTitle>Pick</DialogTitle>
					<MultiSelectCombobox
						commandProps={{ label: "Templates" }}
						options={options}
					/>
				</DialogContent>
			</Dialog>,
		);

		await user.click(screen.getByRole("combobox", { name: "Templates" }));
		await user.keyboard("{Escape}");
		expect(onOpenChange).not.toHaveBeenCalled();

		await user.keyboard("{Escape}");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});

	it("names the buttons that remove selected options", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<MultiSelectCombobox
				commandProps={{ label: "Templates" }}
				options={options}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("combobox", { name: "Templates" }));
		await user.click(screen.getByRole("option", { name: "Docker" }));
		await user.click(screen.getByRole("option", { name: "Kubernetes" }));
		await user.click(screen.getByRole("button", { name: "Remove Docker" }));
		expect(onChange).toHaveBeenLastCalledWith([options[1]]);

		await user.click(screen.getByRole("button", { name: "Clear all" }));
		expect(onChange).toHaveBeenLastCalledWith([]);
	});

	it("keeps focus in the field and adds nothing when Enter removes an option", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<MultiSelectCombobox
				commandProps={{ label: "Templates" }}
				options={options}
				onChange={onChange}
			/>,
		);
		const input = screen.getByRole("combobox", { name: "Templates" });
		await user.click(input);
		await user.click(screen.getByRole("option", { name: "Docker" }));
		await user.click(screen.getByRole("option", { name: "Kubernetes" }));

		screen.getByRole("button", { name: "Remove Docker" }).focus();
		await user.keyboard("{ArrowDown}{Enter}");

		expect(onChange).toHaveBeenLastCalledWith([options[1]]);
		expect(input).toHaveFocus();
	});
});

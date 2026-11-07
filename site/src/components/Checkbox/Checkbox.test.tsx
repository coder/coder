import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useId, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
	it("toggles an uncontrolled value through the label and keyboard", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		function Field() {
			const id = useId();
			return (
				<>
					<label htmlFor={id}>Accept terms</label>
					<Checkbox id={id} onCheckedChange={onCheckedChange} />
				</>
			);
		}
		render(<Field />);
		await user.click(screen.getByText("Accept terms"));
		await user.keyboard(" ");
		expect(onCheckedChange.mock.calls.map(([checked]) => checked)).toEqual([
			true,
			false,
		]);
	});

	it("changes an indeterminate value to checked", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		function Field() {
			const [checked, setChecked] = useState<boolean | "indeterminate">(
				"indeterminate",
			);
			return (
				<Checkbox
					aria-label="Select all"
					checked={checked}
					onCheckedChange={(next) => {
						setChecked(next);
						onCheckedChange(next);
					}}
				/>
			);
		}
		render(<Field />);
		await user.click(screen.getByRole("checkbox", { name: "Select all" }));
		await user.keyboard(" ");
		expect(onCheckedChange.mock.calls).toEqual([[true], [false]]);
	});

	it("ignores disabled interaction", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		render(
			<Checkbox
				aria-label="Accept terms"
				disabled
				onCheckedChange={onCheckedChange}
			/>,
		);
		await user.click(screen.getByRole("checkbox", { name: "Accept terms" }));
		expect(onCheckedChange).not.toHaveBeenCalled();
	});
});

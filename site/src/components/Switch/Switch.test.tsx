import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useId, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { Switch } from "./Switch";

describe("Switch", () => {
	it("toggles an uncontrolled switch with the keyboard", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		render(
			<Switch aria-label="Notifications" onCheckedChange={onCheckedChange} />,
		);
		await user.tab();
		await user.keyboard(" ");
		await user.keyboard(" ");
		expect(onCheckedChange.mock.calls.map(([checked]) => checked)).toEqual([
			true,
			false,
		]);
	});

	it("updates a controlled form value through its label without submitting", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn((event: React.FormEvent) => event.preventDefault());
		function Form() {
			const id = useId();
			const [checked, setChecked] = useState(false);
			return (
				<form aria-label="Settings" onSubmit={onSubmit}>
					<label htmlFor={id}>Notifications</label>
					<Switch
						id={id}
						name="notifications"
						checked={checked}
						onCheckedChange={setChecked}
					/>
				</form>
			);
		}
		render(<Form />);
		const form = screen.getByRole<HTMLFormElement>("form", {
			name: "Settings",
		});
		await user.click(screen.getByText("Notifications"));
		expect(new FormData(form).get("notifications")).toBe("on");
		await user.click(screen.getByRole("switch", { name: "Notifications" }));
		expect(new FormData(form).get("notifications")).toBeNull();
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it("ignores interaction when disabled", async () => {
		const user = userEvent.setup();
		const onCheckedChange = vi.fn();
		render(
			<Switch
				aria-label="Notifications"
				disabled
				onCheckedChange={onCheckedChange}
			/>,
		);
		await user.click(screen.getByRole("switch", { name: "Notifications" }));
		await user.tab();
		await user.keyboard(" ");
		expect(onCheckedChange).not.toHaveBeenCalled();
	});
});

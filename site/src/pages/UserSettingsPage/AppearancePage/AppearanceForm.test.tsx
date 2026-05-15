import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { AppearanceForm } from "./AppearanceForm";

const makeSettings = (
	overrides: Partial<UserAppearanceSettings> = {},
): UserAppearanceSettings => ({
	theme_preference: "dark",
	theme_mode: "single",
	theme_light: "light",
	theme_dark: "dark",
	terminal_font: "",
	...overrides,
});

const renderAppearanceForm = (children: ReactNode) => {
	return render(
		<TooltipProvider delayDuration={100}>{children}</TooltipProvider>,
	);
};

describe("appearance form", () => {
	it("submits changes immediately and keeps the local draft responsive", () => {
		const onSubmit = vi.fn();

		renderAppearanceForm(
			<AppearanceForm
				activeScheme="dark"
				initialValues={makeSettings()}
				onSubmit={onSubmit}
			/>,
		);

		fireEvent.click(screen.getByText("Fira Code"));

		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(onSubmit).toHaveBeenLastCalledWith({
			theme_preference: "dark",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "fira-code",
		});

		fireEvent.click(screen.getByRole("radio", { name: /light default/i }));

		expect(onSubmit).toHaveBeenCalledTimes(2);
		expect(onSubmit).toHaveBeenLastCalledWith({
			theme_preference: "light",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "fira-code",
		});
		expect(screen.getByRole("radio", { name: /light default/i })).toBeChecked();
	});

	it("resyncs the local draft when initialValues change between renders", () => {
		const onSubmit = vi.fn();
		const { rerender } = render(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={makeSettings({
						theme_preference: "dark",
						theme_mode: "single",
						theme_light: "light",
						theme_dark: "dark",
					})}
					onSubmit={onSubmit}
				/>
			</TooltipProvider>,
		);

		expect(screen.getByRole("radio", { name: /dark default/i })).toBeChecked();

		rerender(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={makeSettings({
						theme_preference: "light",
						theme_mode: "single",
						theme_light: "light",
						theme_dark: "dark",
					})}
					onSubmit={onSubmit}
				/>
			</TooltipProvider>,
		);

		expect(screen.getByRole("radio", { name: /light default/i })).toBeChecked();
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it("preserves the local draft while an update is pending", () => {
		const onSubmit = vi.fn();
		const { rerender } = render(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={makeSettings()}
					isUpdating
					onSubmit={onSubmit}
				/>
			</TooltipProvider>,
		);

		fireEvent.click(screen.getByRole("radio", { name: /light default/i }));
		expect(screen.getByRole("radio", { name: /light default/i })).toBeChecked();

		rerender(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={makeSettings()}
					isUpdating
					onSubmit={onSubmit}
				/>
			</TooltipProvider>,
		);

		expect(screen.getByRole("radio", { name: /light default/i })).toBeChecked();

		rerender(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={makeSettings()}
					onSubmit={onSubmit}
				/>
			</TooltipProvider>,
		);

		expect(screen.getByRole("radio", { name: /dark default/i })).toBeChecked();
	});
});

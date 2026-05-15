import { act, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { AppearanceForm } from "./AppearanceForm";

const putResponse = (
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
	it("queues the latest draft while an update is in flight", async () => {
		const submitResolvers: Array<(value: UserAppearanceSettings) => void> = [];
		const onSubmit = vi.fn(
			() =>
				new Promise<UserAppearanceSettings>((resolve) => {
					submitResolvers.push(resolve);
				}),
		);

		renderAppearanceForm(
			<AppearanceForm
				activeScheme="dark"
				initialValues={putResponse()}
				onSubmit={onSubmit}
			/>,
		);

		fireEvent.click(screen.getByText("Fira Code"));
		fireEvent.click(screen.getByRole("radio", { name: /dark tritanopia/i }));
		fireEvent.click(screen.getByRole("radio", { name: /light default/i }));

		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(onSubmit).toHaveBeenLastCalledWith({
			theme_preference: "dark",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "fira-code",
		});

		await act(async () => {
			submitResolvers[0]?.(putResponse({ terminal_font: "fira-code" }));
			await Promise.resolve();
		});

		expect(onSubmit).toHaveBeenCalledTimes(2);
		expect(onSubmit).toHaveBeenLastCalledWith({
			theme_preference: "light",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "fira-code",
		});
		await act(async () => {
			submitResolvers[1]?.(
				putResponse({
					theme_preference: "light",
					terminal_font: "fira-code",
				}),
			);
			await Promise.resolve();
		});
	});

	it("rolls back local draft and releases submit guard on failure", async () => {
		let rejectSubmit: ((error: unknown) => void) | undefined;
		const onSubmit = vi.fn(
			() =>
				new Promise<UserAppearanceSettings>((_, reject) => {
					rejectSubmit = reject;
				}),
		);

		renderAppearanceForm(
			<AppearanceForm
				activeScheme="dark"
				initialValues={putResponse()}
				onSubmit={onSubmit}
			/>,
		);

		const lightDefault = screen.getByRole("radio", {
			name: /light default/i,
		});
		fireEvent.click(lightDefault);
		expect(lightDefault).toBeChecked();

		await act(async () => {
			rejectSubmit?.(new Error("failed"));
			await Promise.resolve();
		});

		expect(screen.getByRole("radio", { name: /dark default/i })).toBeChecked();

		fireEvent.click(screen.getByRole("radio", { name: /light default/i }));
		expect(onSubmit).toHaveBeenCalledTimes(2);
		await act(async () => {
			rejectSubmit?.(new Error("failed again"));
			await Promise.resolve();
		});
	});

	it("resyncs the local draft when initialValues change between renders", () => {
		const onSubmit = vi.fn(() => Promise.resolve(putResponse()));
		const { rerender } = render(
			<TooltipProvider delayDuration={100}>
				<AppearanceForm
					activeScheme="dark"
					initialValues={putResponse({
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
					initialValues={putResponse({
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
});

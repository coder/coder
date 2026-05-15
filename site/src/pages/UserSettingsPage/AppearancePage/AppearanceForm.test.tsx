import { act, fireEvent, render, screen } from "@testing-library/react";
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
				initialValues={makeSettings()}
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
			submitResolvers[0]?.(makeSettings({ terminal_font: "fira-code" }));
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
				makeSettings({
					theme_preference: "light",
					terminal_font: "fira-code",
				}),
			);
			await Promise.resolve();
		});
	});

	it("submits the queued draft after an earlier update fails", async () => {
		const submitResolvers: Array<{
			resolve: (value: UserAppearanceSettings) => void;
			reject: (error: unknown) => void;
		}> = [];
		const onSubmit = vi.fn(
			() =>
				new Promise<UserAppearanceSettings>((resolve, reject) => {
					submitResolvers.push({ resolve, reject });
				}),
		);

		renderAppearanceForm(
			<AppearanceForm
				activeScheme="dark"
				initialValues={makeSettings()}
				onSubmit={onSubmit}
			/>,
		);

		fireEvent.click(screen.getByText("Fira Code"));
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
			submitResolvers[0]?.reject(new Error("failed"));
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
		expect(screen.getByRole("radio", { name: /light default/i })).toBeChecked();

		await act(async () => {
			submitResolvers[1]?.resolve(
				makeSettings({
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
				initialValues={makeSettings()}
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
		expect(screen.getByRole("radio", { name: /dark default/i })).toBeChecked();
	});

	it("resyncs the local draft when initialValues change between renders", () => {
		const onSubmit = vi.fn(() => Promise.resolve(makeSettings()));
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
});

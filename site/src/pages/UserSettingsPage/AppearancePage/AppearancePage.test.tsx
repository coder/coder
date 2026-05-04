import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { API } from "#/api/api";
import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { AppearanceForm } from "./AppearanceForm";
import AppearancePage from "./AppearancePage";

// Helper for building a mock PUT response. The shape is a full
// UserAppearanceSettings so the TS contract matches the API method.
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

describe("appearance page", () => {
	it("switches to single theme mode and picks Light default", async () => {
		renderWithAuth(<AppearancePage />);

		vi.spyOn(API, "updateAppearanceSettings").mockResolvedValue(
			putResponse({
				terminal_font: "geist-mono",
				theme_preference: "light",
			}),
		);

		// The initial state (from MockUserAppearanceSettings) is single
		// mode with `dark` selected. Click the Light default tile and
		// assert the submit payload.
		const lightDefault = await screen.findByText("Light default", {
			exact: true,
		});
		await userEvent.click(lightDefault);

		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			theme_preference: "light",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "geist-mono",
		});
	});

	it("switches to sync mode and sends the expected payload", async () => {
		renderWithAuth(<AppearancePage />);

		vi.spyOn(API, "updateAppearanceSettings").mockResolvedValue(
			putResponse({
				terminal_font: "geist-mono",
				theme_preference: "dark",
				theme_mode: "sync",
			}),
		);

		const dropdown = await screen.findByRole("combobox", {
			name: /theme mode/i,
		});
		await userEvent.click(dropdown);
		const syncOption = await screen.findByRole("option", {
			name: /sync with system/i,
		});
		await userEvent.click(syncOption);

		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			theme_preference: "dark",
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "geist-mono",
		});
	});

	it("keeps the dropdown on sync after a legacy server drops the new fields", async () => {
		renderWithAuth(<AppearancePage />);

		vi.spyOn(API, "updateAppearanceSettings").mockResolvedValue({
			theme_preference: "dark",
			terminal_font: "geist-mono",
		} as UserAppearanceSettings);

		const dropdown = await screen.findByRole("combobox", {
			name: /theme mode/i,
		});
		expect(dropdown).toHaveTextContent(/single theme/i);

		await userEvent.click(dropdown);
		await userEvent.click(
			await screen.findByRole("option", { name: /sync with system/i }),
		);

		await screen.findByRole("radiogroup", { name: /light theme options/i });
		expect(dropdown).toHaveTextContent(/sync with system/i);
	});

	it("updates the terminal font", async () => {
		renderWithAuth(<AppearancePage />);

		vi.spyOn(API, "updateAppearanceSettings").mockResolvedValue(
			putResponse({
				terminal_font: "fira-code",
				theme_preference: "dark",
			}),
		);

		const firaCode = await screen.findByText("Fira Code");
		await userEvent.click(firaCode);

		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			theme_preference: "dark",
			theme_mode: "single",
			theme_light: "light",
			theme_dark: "dark",
			terminal_font: "fira-code",
		});
	});

	it("renders every concrete theme in each sync mode slot", () => {
		renderAppearanceForm(
			<AppearanceForm
				activeScheme="light"
				initialValues={putResponse({
					theme_mode: "sync",
					theme_light: "light",
					theme_dark: "dark",
				})}
				onSubmit={vi.fn()}
			/>,
		);

		const lightOptions = screen.getByRole("radiogroup", {
			name: /light theme options/i,
		});
		const darkOptions = screen.getByRole("radiogroup", {
			name: /dark theme options/i,
		});

		expect(within(lightOptions).getAllByRole("radio")).toHaveLength(6);
		expect(within(darkOptions).getAllByRole("radio")).toHaveLength(6);
	});

	it("allows a dark concrete theme in the light sync slot", () => {
		const onSubmit = vi.fn(() => Promise.resolve(putResponse()));
		renderAppearanceForm(
			<AppearanceForm
				activeScheme="light"
				initialValues={putResponse({
					theme_preference: "light",
					theme_mode: "sync",
					theme_light: "light",
					theme_dark: "dark",
				})}
				onSubmit={onSubmit}
			/>,
		);

		const lightOptions = screen.getByRole("radiogroup", {
			name: /light theme options/i,
		});
		const darkTritanopia = within(lightOptions).getByRole("radio", {
			name: /dark tritanopia/i,
		});
		fireEvent.click(darkTritanopia);

		expect(darkTritanopia).toBeChecked();
		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(onSubmit).toHaveBeenCalledWith({
			theme_preference: "dark-tritan",
			theme_mode: "sync",
			theme_light: "dark-tritan",
			theme_dark: "dark",
			terminal_font: "geist-mono",
		});
	});

	it("keeps the legacy mirror on the active light slot in sync mode", () => {
		const onSubmit = vi.fn(() => Promise.resolve(putResponse()));
		renderAppearanceForm(
			<AppearanceForm
				activeScheme="light"
				initialValues={putResponse({
					theme_preference: "dark",
					theme_mode: "sync",
					theme_light: "light-tritan",
					theme_dark: "dark",
				})}
				onSubmit={onSubmit}
			/>,
		);

		const darkOptions = screen.getByRole("radiogroup", {
			name: /dark theme options/i,
		});
		fireEvent.click(
			within(darkOptions).getByRole("radio", { name: /dark tritanopia/i }),
		);

		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(onSubmit).toHaveBeenCalledWith({
			theme_preference: "light-tritan",
			theme_mode: "sync",
			theme_light: "light-tritan",
			theme_dark: "dark-tritan",
			terminal_font: "geist-mono",
		});
	});

	it("ignores repeated submits while an update is in flight", async () => {
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

		fireEvent.click(screen.getByRole("radio", { name: /light default/i }));
		fireEvent.click(screen.getByRole("radio", { name: /dark default/i }));

		expect(onSubmit).toHaveBeenCalledTimes(1);
		submitResolvers[0]?.(putResponse());
		await Promise.resolve();

		fireEvent.click(screen.getByRole("radio", { name: /dark default/i }));
		expect(onSubmit).toHaveBeenCalledTimes(2);
		submitResolvers[1]?.(putResponse());
		await Promise.resolve();
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

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith(
			expect.objectContaining({
				theme_mode: "single",
				theme_preference: "light",
			}),
		);
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
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith(
			expect.objectContaining({
				theme_mode: "sync",
				theme_light: "light",
				theme_dark: "dark",
			}),
		);
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
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith(
			expect.objectContaining({
				terminal_font: "fira-code",
			}),
		);
	});
});

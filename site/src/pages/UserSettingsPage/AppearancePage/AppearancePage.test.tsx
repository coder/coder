import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { MockUser } from "testHelpers/entities";
import { renderWithAuth } from "testHelpers/renderHelpers";
import { AppearancePage } from "./AppearancePage";

describe("appearance page", () => {
	it("does nothing when selecting current theme", async () => {
		renderWithAuth(<AppearancePage />);

		jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
			...MockUser,
			theme_preference: "dark",
			terminal_font: "fira-code",
			terminal_font_size: 16,
		});

		const dark = await screen.findByText("Dark");
		await userEvent.click(dark);

		// Check if the API was called correctly
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(0);
	});

	it("changes theme to light", async () => {
		renderWithAuth(<AppearancePage />);

		jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
			...MockUser,
			terminal_font: "ibm-plex-mono",
			theme_preference: "light",
			terminal_font_size: 16,
		});

		const light = await screen.findByText("Light");
		await userEvent.click(light);

		// Check if the API was called correctly
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			terminal_font: "ibm-plex-mono",
			theme_preference: "light",
			terminal_font_size: 16,
		});
	});

	it("changes font to fira code", async () => {
		renderWithAuth(<AppearancePage />);

		jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
			...MockUser,
			terminal_font: "fira-code",
			theme_preference: "dark",
			terminal_font_size: 16,
		});

		const firaCode = await screen.findByText("Fira Code");
		await userEvent.click(firaCode);

		// Check if the API was called correctly
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			terminal_font: "fira-code",
			theme_preference: "dark",
			terminal_font_size: 16,
		});
	});

	it("changes font to fira code, then back to web terminal font", async () => {
		renderWithAuth(<AppearancePage />);

		// given
		jest
			.spyOn(API, "updateAppearanceSettings")
			.mockResolvedValueOnce({
				...MockUser,
				terminal_font: "fira-code",
				theme_preference: "dark",
				terminal_font_size: 16,
			})
			.mockResolvedValueOnce({
				...MockUser,
				terminal_font: "ibm-plex-mono",
				theme_preference: "dark",
				terminal_font_size: 16,
			});

		// when
		const firaCode = await screen.findByText("Fira Code");
		await userEvent.click(firaCode);

		// then
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			terminal_font: "fira-code",
			theme_preference: "dark",
			terminal_font_size: 16,
		});

		// when
		const ibmPlex = await screen.findByText("Web Terminal Font");
		await userEvent.click(ibmPlex);

		// then
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(2);
		expect(API.updateAppearanceSettings).toHaveBeenNthCalledWith(2, {
			terminal_font: "ibm-plex-mono",
			theme_preference: "dark",
			terminal_font_size: 16,
		});
	});
});

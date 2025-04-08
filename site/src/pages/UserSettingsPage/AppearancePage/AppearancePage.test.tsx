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
		});

		const light = await screen.findByText("Light");
		await userEvent.click(light);

		// Check if the API was called correctly
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			terminal_font: "ibm-plex-mono",
			theme_preference: "light",
		});
	});

	it("changes font to fira code", async () => {
		renderWithAuth(<AppearancePage />);

		jest.spyOn(API, "updateAppearanceSettings").mockResolvedValueOnce({
			...MockUser,
			terminal_font: "fira-code",
			theme_preference: "dark",
		});

		const ibmPlex = await screen.findByText("Fira Code");
		await userEvent.click(ibmPlex);

		// Check if the API was called correctly
		expect(API.updateAppearanceSettings).toHaveBeenCalledTimes(1);
		expect(API.updateAppearanceSettings).toHaveBeenCalledWith({
			terminal_font: "fira-code",
			theme_preference: "dark",
		});
	});
});

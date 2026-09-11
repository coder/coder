import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { isMac } from "#/utils/platform";
import {
	VIM_NAVIGATION_MODIFIER_STORAGE_KEY,
	VIM_NAVIGATION_STORAGE_KEY,
} from "../hooks/useVimNavigation";
import { ChatVimNavigationSettings } from "./ChatVimNavigationSettings";

vi.mock("#/utils/platform", async (importOriginal) => ({
	...(await importOriginal<typeof import("#/utils/platform")>()),
	isMac: vi.fn(),
}));

const isMacMock = vi.mocked(isMac);

describe("ChatVimNavigationSettings", () => {
	afterEach(() => {
		localStorage.removeItem(VIM_NAVIGATION_STORAGE_KEY);
		localStorage.removeItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY);
	});

	it("stores the toggle state", async () => {
		isMacMock.mockReturnValue(false);
		renderComponent(<ChatVimNavigationSettings />);

		await userEvent.click(
			screen.getByRole("switch", { name: "Vim-style chat navigation" }),
		);

		expect(localStorage.getItem(VIM_NAVIGATION_STORAGE_KEY)).toBe("true");
	});

	it("stores the selected modifier", async () => {
		isMacMock.mockReturnValue(false);
		renderComponent(<ChatVimNavigationSettings />);

		await userEvent.click(
			screen.getByRole("combobox", { name: "Vim navigation modifier" }),
		);
		await userEvent.click(await screen.findByRole("option", { name: "Alt" }));

		expect(localStorage.getItem(VIM_NAVIGATION_MODIFIER_STORAGE_KEY)).toBe(
			"alt",
		);
	});
});

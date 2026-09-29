import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ChatBoardSettings } from "./ChatBoardSettings";

describe("ChatBoardSettings", () => {
	afterEach(() => {
		localStorage.clear();
	});

	it("toggles the stored flag from the switch", async () => {
		const user = userEvent.setup();
		renderComponent(<ChatBoardSettings />);
		const toggle = screen.getByRole("switch", { name: "Chat board" });
		expect(toggle).toHaveAttribute("aria-checked", "false");

		await user.click(toggle);
		expect(toggle).toHaveAttribute("aria-checked", "true");
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("true");

		await user.click(toggle);
		expect(toggle).toHaveAttribute("aria-checked", "false");
		expect(localStorage.getItem("agents.exp.chat-board")).toBe("false");
	});
});

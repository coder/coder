import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { MockPngBase64 } from "#/testHelpers/files";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ToolResultImage } from "./ToolResultImage";

describe("ToolResultImage", () => {
	it("returns keyboard focus to the image once its preview closes", async () => {
		const user = userEvent.setup();
		renderComponent(
			<ToolResultImage
				data={MockPngBase64}
				mimeType="image/png"
				alt="Screenshot"
			/>,
		);

		const image = screen.getByRole("button", { name: "Screenshot" });
		image.focus();
		await user.keyboard("{Enter}");
		await screen.findByRole("dialog");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(image).toHaveFocus());
	});
});

import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { DecorativeImage } from "./DecorativeImage";
import { ExternalImage } from "./ExternalImage";

describe("ExternalImage accessibility", () => {
	it("exposes informative images by role and accessible name", () => {
		renderComponent(<ExternalImage src="/icon/github.svg" alt="GitHub logo" />);

		expect(
			screen.getByRole("img", { name: "GitHub logo" }),
		).toBeInTheDocument();
	});

	it("excludes decorative images from role-based image queries", () => {
		renderComponent(<DecorativeImage src="/icon/github.svg" />);

		expect(screen.queryByRole("img")).not.toBeInTheDocument();
	});
});

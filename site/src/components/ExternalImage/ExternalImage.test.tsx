import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { ExternalImage } from "./ExternalImage";

describe(ExternalImage.name, () => {
	it("renders informative alt text", () => {
		renderComponent(<ExternalImage src="/icon/github.svg" alt="GitHub logo" />);

		expect(
			screen.getByRole("img", { name: "GitHub logo" }),
		).toBeInTheDocument();
	});

	it("passes standard image props through to the underlying element", () => {
		renderComponent(
			<ExternalImage
				alt="GitHub logo"
				src="/icon/github.svg"
				width={20}
				height={20}
				loading="lazy"
				data-testid="external-image"
			/>,
		);

		const image = screen.getByTestId("external-image");
		expect(image).toHaveAttribute("src", "/icon/github.svg");
		expect(image).toHaveAttribute("width", "20");
		expect(image).toHaveAttribute("height", "20");
		expect(image).toHaveAttribute("loading", "lazy");
	});
});

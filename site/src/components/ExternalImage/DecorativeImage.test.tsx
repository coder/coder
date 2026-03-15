import { renderComponent } from "testHelpers/renderHelpers";
import { screen } from "@testing-library/react";
import { DecorativeImage } from "./DecorativeImage";

describe(DecorativeImage.name, () => {
	it("renders decorative semantics", () => {
		renderComponent(
			<DecorativeImage src="/icon/github.svg" data-testid="decorative-image" />,
		);

		const image = screen.getByTestId("decorative-image");
		expect(image).toHaveAttribute("alt", "");
		expect(image).toHaveAttribute("aria-hidden", "true");
	});
});

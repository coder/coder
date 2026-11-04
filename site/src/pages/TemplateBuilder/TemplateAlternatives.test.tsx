import { screen } from "@testing-library/react";
import { render } from "#/testHelpers/renderHelpers";
import { TemplateAlternatives } from "./TemplateAlternatives";

describe(TemplateAlternatives.name, () => {
	it("announces which alternatives open in a new tab", async () => {
		render(<TemplateAlternatives />);

		expect(
			await screen.findByRole("link", { name: /^Start from scratch/ }),
		).toHaveAccessibleName(/^Start from scratch\s*\(opens in new tab\)$/);
		expect(
			screen.getByRole("link", { name: /^Upload an existing template/ }),
		).toHaveAccessibleName("Upload an existing template");
	});
});

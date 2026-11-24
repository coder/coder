import { render, screen } from "@testing-library/react";
import { HelpPopoverLink } from "./HelpPopover";

describe(HelpPopoverLink.name, () => {
	it("announces that the link opens in a new tab", () => {
		render(
			<HelpPopoverLink href="https://docs.example.com">
				Read the docs
			</HelpPopoverLink>,
		);

		expect(screen.getByRole("link")).toHaveAccessibleName(
			/^Read the docs\s*\(opens in new tab\)$/,
		);
	});
});

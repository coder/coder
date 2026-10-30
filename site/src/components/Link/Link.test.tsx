import { render, screen } from "@testing-library/react";
import { Link } from "./Link";

describe("Link", () => {
	it("announces that an external link opens in a new tab", () => {
		render(
			<Link href="https://docs.example.com" target="_blank">
				View docs
			</Link>,
		);

		expect(screen.getByRole("link")).toHaveAccessibleName(
			/^View docs\s*\(opens in new tab\)$/,
		);
	});

	it("does not announce a new tab for a same-tab link", () => {
		render(<Link href="https://docs.example.com">View docs</Link>);

		expect(screen.getByRole("link")).toHaveAccessibleName("View docs");
	});

	it("does not announce a new tab when the external icon is hidden", () => {
		render(
			<Link
				href="https://docs.example.com"
				target="_blank"
				showExternalIcon={false}
			>
				View docs
			</Link>,
		);

		expect(screen.getByRole("link")).toHaveAccessibleName("View docs");
	});
});

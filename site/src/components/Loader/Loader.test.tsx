import { render, screen } from "@testing-library/react";
import { Loader } from "./Loader";

describe("Loader", () => {
	it("announces loading status politely", () => {
		render(<Loader />);

		expect(screen.getByRole("status")).toHaveAttribute("aria-live", "polite");
		expect(screen.getByLabelText("Loading")).toBeInTheDocument();
	});

	it("applies custom spinner labels when provided", () => {
		render(<Loader label="Loading workspace resources" />);

		expect(
			screen.getByLabelText("Loading workspace resources"),
		).toBeInTheDocument();
	});
});

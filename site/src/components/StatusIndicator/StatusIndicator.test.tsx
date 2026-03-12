import { render, screen } from "@testing-library/react";
import { StatusIndicatorDot } from "./StatusIndicator";

describe("StatusIndicatorDot", () => {
	it("does not apply the pulse class by default", () => {
		render(<StatusIndicatorDot data-testid="dot" />);

		expect(screen.getByTestId("dot")).not.toHaveClass(
			"animate-status-dot-pulse",
		);
	});

	it("applies the pulse class when pulse is true", () => {
		render(<StatusIndicatorDot data-testid="dot" pulse />);

		expect(screen.getByTestId("dot")).toHaveClass("animate-status-dot-pulse");
	});

	it("does not apply the pulse class when pulse is false", () => {
		render(<StatusIndicatorDot data-testid="dot" pulse={false} />);

		expect(screen.getByTestId("dot")).not.toHaveClass(
			"animate-status-dot-pulse",
		);
	});

	it("renders a failed variant without pulse", () => {
		render(<StatusIndicatorDot data-testid="dot" variant="failed" />);

		expect(screen.getByTestId("dot")).not.toHaveClass(
			"animate-status-dot-pulse",
		);
	});

	it("renders a failed variant with pulse when requested", () => {
		render(<StatusIndicatorDot data-testid="dot" pulse variant="failed" />);

		expect(screen.getByTestId("dot")).toHaveClass("animate-status-dot-pulse");
	});
});

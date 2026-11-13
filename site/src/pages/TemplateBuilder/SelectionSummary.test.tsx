import { screen } from "@testing-library/react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { SelectionSummary } from "./SelectionSummary";

const renderSummary = (currentStep: number, maxReachedStep: number) =>
	renderComponent(
		<SelectionSummary
			currentStep={currentStep}
			maxReachedStep={maxReachedStep}
			onNavigateStep={vi.fn()}
			onNavigateModule={vi.fn()}
		/>,
	);

describe(SelectionSummary.name, () => {
	it("marks only the current step with aria-current", () => {
		renderSummary(2, 3);

		expect(
			screen.getByRole("button", { name: "Go to Modules" }),
		).toHaveAttribute("aria-current", "step");
		expect(
			screen.getByRole("button", { name: "Go to Base Template" }),
		).not.toHaveAttribute("aria-current");
		expect(
			screen.getByRole("button", { name: "Go to Customizations" }),
		).not.toHaveAttribute("aria-current");
	});

	it("marks the current step when it is not navigable", () => {
		renderSummary(1, 0);

		expect(screen.queryAllByRole("button")).toHaveLength(0);
		expect(screen.getByText("Base Template").parentElement).toHaveAttribute(
			"aria-current",
			"step",
		);
	});
});

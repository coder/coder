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
			screen.getByRole("button", { name: "Go to Base template" }),
		).not.toHaveAttribute("aria-current");
		expect(
			screen.getByRole("button", { name: "Go to Customizations" }),
		).not.toHaveAttribute("aria-current");
	});

	it("announces the selected base template and module count", () => {
		renderComponent(
			<SelectionSummary
				currentStep={2}
				maxReachedStep={2}
				selectedTemplate={{ name: "Docker Containers" }}
				selectedModules={[
					{ id: "git-clone", name: "Git clone", iconUrl: "/icon/git.svg" },
				]}
				onNavigateStep={vi.fn()}
				onNavigateModule={vi.fn()}
			/>,
		);

		expect(screen.getByRole("status")).toHaveTextContent(
			"Base template: Docker Containers. 1 module selected.",
		);
	});

	it("marks the current step when it is not navigable", () => {
		renderSummary(1, 0);

		expect(screen.queryAllByRole("button")).toHaveLength(0);
		expect(screen.getByText("Base template").parentElement).toHaveAttribute(
			"aria-current",
			"step",
		);
	});
});

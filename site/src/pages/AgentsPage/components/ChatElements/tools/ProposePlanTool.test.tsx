import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { render } from "#/testHelpers/renderHelpers";
import { ProposePlanTool } from "./ProposePlanTool";

const samplePlan =
	"# Implementation Plan\n\nRefactor the authentication module.";

const planFileID = "test-file-id-plan";

const renderTool = (onImplementPlan: () => void) => {
	render(
		<ProposePlanTool
			path="/home/coder/PLAN.md"
			status="completed"
			isError={false}
			fileID={planFileID}
			onImplementPlan={onImplementPlan}
		/>,
	);
};

describe("ProposePlanTool", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("calls onImplementPlan once when Implement is clicked", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
		const onImplementPlan = vi.fn();

		renderTool(onImplementPlan);

		const implementButton = await screen.findByRole("button", {
			name: "Implement plan",
		});
		await user.click(implementButton);

		expect(onImplementPlan).toHaveBeenCalledTimes(1);
	});

	it("copies the fetched plan text to the clipboard", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatFileText").mockResolvedValue(samplePlan);
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined);

		renderTool(vi.fn());

		await screen.findByText("Implementation Plan");
		await user.click(screen.getByRole("button", { name: "Copy plan" }));

		expect(writeText).toHaveBeenCalledTimes(1);
		expect(writeText).toHaveBeenCalledWith(samplePlan);
	});
});

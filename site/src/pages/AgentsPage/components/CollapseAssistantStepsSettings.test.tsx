import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { CollapseAssistantStepsSettings } from "./CollapseAssistantStepsSettings";

describe("CollapseAssistantStepsSettings", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("saves the toggled preference and reflects the saved value", async () => {
		let collapseAssistantSteps = false;
		vi.spyOn(API, "getUserPreferenceSettings").mockImplementation(async () => ({
			...MockUserPreferenceSettings,
			collapse_assistant_steps: collapseAssistantSteps,
		}));
		const update = vi
			.spyOn(API, "updateUserPreferenceSettings")
			.mockImplementation(async (req) => {
				collapseAssistantSteps =
					req.collapse_assistant_steps ?? collapseAssistantSteps;
				return {
					...MockUserPreferenceSettings,
					collapse_assistant_steps: collapseAssistantSteps,
				};
			});
		render(<CollapseAssistantStepsSettings />);
		const toggle = await screen.findByRole("switch", {
			name: "Collapse assistant steps",
		});
		await waitFor(() => expect(toggle).toBeEnabled());

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(update).toHaveBeenCalledWith({ collapse_assistant_steps: true });
			expect(toggle).toBeChecked();
		});

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(update).toHaveBeenLastCalledWith({
				collapse_assistant_steps: false,
			});
			expect(toggle).not.toBeChecked();
		});
	});

	it("disables the switch and reports a failed preference load", async () => {
		vi.spyOn(API, "getUserPreferenceSettings").mockRejectedValue(
			new Error("boom"),
		);
		render(<CollapseAssistantStepsSettings />);
		await screen.findByText(
			"Failed to load your collapse assistant steps preference.",
		);
		expect(
			screen.getByRole("switch", { name: "Collapse assistant steps" }),
		).toBeDisabled();
	});
});

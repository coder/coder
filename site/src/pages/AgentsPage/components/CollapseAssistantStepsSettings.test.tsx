import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { API } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { CollapseAssistantStepsSettings } from "./CollapseAssistantStepsSettings";

describe("CollapseAssistantStepsSettings", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("saves the toggled preference and refetches the saved value", async () => {
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
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(preferenceSettingsKey, {
			...MockUserPreferenceSettings,
			collapse_assistant_steps: false,
		});
		renderComponent(
			<QueryClientProvider client={queryClient}>
				<CollapseAssistantStepsSettings />
			</QueryClientProvider>,
		);
		const toggle = screen.getByRole("switch", {
			name: "Collapse assistant steps",
		});

		await userEvent.click(toggle);
		await waitFor(() =>
			expect(update).toHaveBeenCalledWith({ collapse_assistant_steps: true }),
		);
		await waitFor(() =>
			expect(queryClient.getQueryData(preferenceSettingsKey)).toMatchObject({
				collapse_assistant_steps: true,
			}),
		);

		await userEvent.click(toggle);
		await waitFor(() =>
			expect(update).toHaveBeenLastCalledWith({
				collapse_assistant_steps: false,
			}),
		);
	});
});

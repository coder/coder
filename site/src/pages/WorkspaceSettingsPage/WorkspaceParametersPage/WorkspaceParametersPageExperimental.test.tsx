import { screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { API } from "#/api/api";
import {
	MockPreviewParameter,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter4,
	MockValidationParameter,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter4,
} from "#/testHelpers/entities";
import {
	renderWithWorkspaceSettingsLayout,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "#/testHelpers/websockets";
import WorkspaceParametersPageExperimental from "./WorkspaceParametersPageExperimental";

describe("WorkspaceParametersPageExperimental", () => {
	const renderWorkspaceParametersPageExperimental = (
		route = `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings`,
	) => {
		return renderWithWorkspaceSettingsLayout(
			<WorkspaceParametersPageExperimental />,
			{
				route,
				path: "/:username/:workspace/settings",
				extraRoutes: [
					{
						// Need this because after submit the user is redirected.
						path: "/:username/:workspace",
						element: <div>Workspace Page</div>,
					},
				],
			},
		);
	};

	beforeEach(() => {
		vi.clearAllMocks();
		vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValueOnce(
			MockWorkspace,
		);
		vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter4,
		]);
		vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter2,
			MockWorkspaceBuildParameter4,
		]);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("does not clobber touched parameters", async () => {
		const [, mockPublisher] = mockDynamicParameterWebSocket([
			{
				...MockPreviewParameter,
				name: MockWorkspaceBuildParameter1.name,
			},
		]);

		renderWorkspaceParametersPageExperimental();
		await waitForLoaderToBeRemoved();

		// Simulate a stale response.
		await act(async () => {
			mockPublisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: 2,
						parameters: [
							{
								...MockPreviewParameter,
								name: MockWorkspaceBuildParameter1.name,
							},
							MockValidationParameter,
						],
					}),
				}),
			);
		});

		// Should have the new field, but keep the existing auto-filled values.
		const form = screen.getByTestId("form");
		await waitFor(() => {
			expect(within(form).getByDisplayValue("50")).toBeInTheDocument();
			expect(
				within(form).getByDisplayValue(MockWorkspaceBuildParameter1.value),
			).toBeInTheDocument();
		});
	});
});

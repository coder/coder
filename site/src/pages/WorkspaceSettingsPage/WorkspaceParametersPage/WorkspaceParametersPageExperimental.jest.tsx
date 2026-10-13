import {
	MockOutdatedStoppedWorkspaceRequireActiveVersion,
	MockPreviewParameter,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter4,
	MockValidationParameter,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter2,
	MockWorkspaceBuildParameter4,
} from "testHelpers/entities";
import {
	renderWithWorkspaceSettingsLayout,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "testHelpers/websockets";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import { act } from "react";
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
		jest.clearAllMocks();
		jest
			.spyOn(API, "getWorkspaceByOwnerAndName")
			.mockResolvedValueOnce(MockWorkspace);
		jest
			.spyOn(API, "getTemplateVersionRichParameters")
			.mockResolvedValueOnce([
				MockTemplateVersionParameter1,
				MockTemplateVersionParameter2,
				MockTemplateVersionParameter4,
			]);
		jest
			.spyOn(API, "getWorkspaceBuildParameters")
			.mockResolvedValueOnce([
				MockWorkspaceBuildParameter1,
				MockWorkspaceBuildParameter2,
				MockWorkspaceBuildParameter4,
			]);
	});

	afterEach(() => {
		jest.useRealTimers();
		jest.restoreAllMocks();
	});

	it("allows updating to a required active version without permission to change versions", async () => {
		const workspace = MockOutdatedStoppedWorkspaceRequireActiveVersion;
		jest
			.mocked(API.getWorkspaceByOwnerAndName)
			.mockReset()
			.mockResolvedValue(workspace);
		jest.spyOn(API, "checkAuthorization").mockResolvedValue({
			readWorkspace: true,
			updateWorkspace: true,
			updateWorkspaceVersion: false,
			deleteFailedWorkspace: false,
		});
		jest
			.mocked(API.getWorkspaceBuildParameters)
			.mockReset()
			.mockResolvedValue([]);
		const postWorkspaceBuild = jest
			.spyOn(API, "postWorkspaceBuild")
			.mockResolvedValue(workspace.latest_build);
		mockDynamicParameterWebSocket([
			{ ...MockPreviewParameter, value: { valid: true, value: "value" } },
		]);

		renderWorkspaceParametersPageExperimental(
			`/@${workspace.owner_name}/${workspace.name}/settings?templateVersionId=${workspace.template_active_version_id}`,
		);
		await waitForLoaderToBeRemoved();

		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /update and restart/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());

		await userEvent.click(submitButton);
		await waitFor(() =>
			expect(postWorkspaceBuild).toHaveBeenCalledWith(
				workspace.id,
				expect.objectContaining({
					template_version_id: workspace.template_active_version_id,
				}),
			),
		);
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
						diagnostics: [],
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

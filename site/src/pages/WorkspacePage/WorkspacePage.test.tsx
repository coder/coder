import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import * as apiModule from "api/api";
import type { TemplateVersionParameter, Workspace } from "api/typesGenerated";
import MockServerSocket from "jest-websocket-mock";
import {
	DashboardContext,
	type DashboardProvider,
} from "modules/dashboard/DashboardProvider";
import { http, HttpResponse } from "msw";
import type { FC } from "react";
import { type Location, useLocation } from "react-router-dom";
import {
	MockAppearanceConfig,
	MockDeploymentConfig,
	MockEntitlements,
	MockFailedWorkspace,
	MockOrganization,
	MockOutdatedWorkspace,
	MockStartingWorkspace,
	MockStoppedWorkspace,
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockUser,
	MockWorkspace,
	MockWorkspaceBuild,
	MockWorkspaceBuildDelete,
} from "testHelpers/entities";
import {
	type RenderWithAuthOptions,
	renderWithAuth,
} from "testHelpers/renderHelpers";
import { server } from "testHelpers/server";
import { WorkspacePage } from "./WorkspacePage";

const { API, MissingBuildParameters } = apiModule;

type RenderWorkspacePageOptions = Omit<RenderWithAuthOptions, "route" | "path">;

// Renders the workspace page and waits for it be loaded
const renderWorkspacePage = async (
	workspace: Workspace,
	options: RenderWorkspacePageOptions = {},
) => {
	jest.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValue(workspace);
	jest.spyOn(API, "getTemplate").mockResolvedValueOnce(MockTemplate);
	jest.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([]);
	jest
		.spyOn(API, "getDeploymentConfig")
		.mockResolvedValueOnce(MockDeploymentConfig);
	jest
		.spyOn(apiModule, "watchWorkspaceAgentLogs")
		.mockImplementation((_, options) => {
			options.onDone?.();
			return new WebSocket("");
		});

	renderWithAuth(<WorkspacePage />, {
		...options,
		route: `/@${workspace.owner_name}/${workspace.name}`,
		path: "/:username/:workspace",
	});

	await screen.findByText(workspace.name);
};

/**
 * Requests and responses related to workspace status are unrelated, so we can't
 * test in the usual way. Instead, test that button clicks produce the correct
 * requests and that responses produce the correct UI.
 *
 * We don't need to test the UI exhaustively because Storybook does that; just
 * enough to prove that the workspaceStatus was calculated correctly.
 */
const testButton = async (
	workspace: Workspace,
	name: string | RegExp,
	actionMock: jest.SpyInstance,
) => {
	await renderWorkspacePage(workspace);
	const workspaceActions = screen.getByTestId("workspace-actions");
	const button = within(workspaceActions).getByRole("button", { name });

	const user = userEvent.setup();
	await user.click(button);
	expect(actionMock).toHaveBeenCalled();
};

afterEach(() => {
	MockServerSocket.clean();
});

describe("WorkspacePage", () => {
	it("requests a delete job when the user presses Delete and confirms", async () => {
		const user = userEvent.setup({ delay: 0 });
		const deleteWorkspaceMock = jest
			.spyOn(API, "deleteWorkspace")
			.mockResolvedValueOnce(MockWorkspaceBuild);
		await renderWorkspacePage(MockWorkspace);

		// open the workspace action popover so we have access to all available ctas
		const trigger = screen.getByTestId("workspace-options-button");
		await user.click(trigger);

		// Click on delete
		const button = await screen.findByTestId("delete-button");
		await user.click(button);

		// Get dialog and confirm
		const dialog = await screen.findByTestId("dialog");
		const labelText = "Workspace name";
		const textField = within(dialog).getByLabelText(labelText);
		await user.type(textField, MockWorkspace.name);
		const confirmButton = within(dialog).getByRole("button", {
			name: "Delete",
			hidden: false,
		});
		await user.click(confirmButton);
		expect(deleteWorkspaceMock).toBeCalled();
	});

	it("orphans the workspace on delete if option is selected", async () => {
		const user = userEvent.setup({ delay: 0 });

		// set permissions
		server.use(
			http.post("/api/v2/authcheck", async () => {
				return HttpResponse.json({
					updateTemplates: true,
					updateWorkspace: true,
					updateTemplate: true,
				});
			}),
		);

		const deleteWorkspaceMock = jest
			.spyOn(API, "deleteWorkspace")
			.mockResolvedValueOnce(MockWorkspaceBuildDelete);
		await renderWorkspacePage(MockFailedWorkspace);

		// open the workspace action popover so we have access to all available ctas
		const trigger = screen.getByTestId("workspace-options-button");
		await user.click(trigger);

		// Click on delete
		const button = await screen.findByTestId("delete-button");
		await user.click(button);

		// Get dialog and enter confirmation text
		const dialog = await screen.findByTestId("dialog");
		const labelText = "Workspace name";
		const textField = within(dialog).getByLabelText(labelText);
		await user.type(textField, MockFailedWorkspace.name);

		// check orphan option
		const orphanCheckbox = within(
			screen.getByTestId("orphan-checkbox"),
		).getByRole("checkbox");

		await user.click(orphanCheckbox);

		// confirm
		const confirmButton = within(dialog).getByRole("button", {
			name: "Delete",
			hidden: false,
		});
		await user.click(confirmButton);
		// arguments are workspace.name, log level (undefined), and orphan
		expect(deleteWorkspaceMock).toBeCalledWith<
			[string, apiModule.DeleteWorkspaceOptions]
		>(MockFailedWorkspace.id, {
			log_level: undefined,
			orphan: true,
		});
	});

	it("requests a start job when the user presses Start", async () => {
		server.use(
			http.get("/api/v2/users/:userId/workspace/:workspaceName", () => {
				return HttpResponse.json(MockStoppedWorkspace);
			}),
		);

		const startWorkspaceMock = jest
			.spyOn(API, "startWorkspace")
			.mockImplementation(() => Promise.resolve(MockWorkspaceBuild));

		await testButton(MockStoppedWorkspace, "Start", startWorkspaceMock);
	});

	it("requests a stop job when the user presses Stop", async () => {
		const stopWorkspaceMock = jest
			.spyOn(API, "stopWorkspace")
			.mockResolvedValueOnce(MockWorkspaceBuild);

		await testButton(MockWorkspace, "Stop", stopWorkspaceMock);
	});

	it("requests a stop when the user presses Restart", async () => {
		const stopWorkspaceMock = jest
			.spyOn(API, "stopWorkspace")
			.mockResolvedValueOnce(MockWorkspaceBuild);

		// Render
		await renderWorkspacePage(MockWorkspace);

		// Actions
		const user = userEvent.setup();
		await user.click(screen.getByTestId("workspace-restart-button"));
		const confirmButton = await screen.findByTestId("confirm-button");
		await user.click(confirmButton);

		// Assertions
		await waitFor(() => {
			expect(stopWorkspaceMock).toBeCalled();
		});
	});

	it("requests cancellation when the user presses Cancel", async () => {
		server.use(
			http.get("/api/v2/users/:userId/workspace/:workspaceName", () => {
				return HttpResponse.json(MockStartingWorkspace);
			}),
		);

		const cancelWorkspaceMock = jest
			.spyOn(API, "cancelWorkspaceBuild")
			.mockImplementation(() => Promise.resolve({ message: "job canceled" }));

		await testButton(MockStartingWorkspace, "Cancel", cancelWorkspaceMock);
	});

	it("requests an update when the user presses Update", async () => {
		// Mocks
		jest
			.spyOn(API, "getWorkspaceByOwnerAndName")
			.mockResolvedValueOnce(MockOutdatedWorkspace);

		const updateWorkspaceMock = jest
			.spyOn(API, "updateWorkspace")
			.mockResolvedValueOnce(MockWorkspaceBuild);

		// Render
		await renderWorkspacePage(MockWorkspace);

		// Actions
		const user = userEvent.setup();
		await user.click(screen.getByTestId("workspace-update-button"));
		const confirmButton = await screen.findByTestId("confirm-button");
		await user.click(confirmButton);

		// Assertions
		await waitFor(() => {
			expect(updateWorkspaceMock).toBeCalled();
		});
	});

	it("updates the parameters when they are missing during update", async () => {
		// Mocks
		jest
			.spyOn(API, "getWorkspaceByOwnerAndName")
			.mockResolvedValueOnce(MockOutdatedWorkspace);
		const updateWorkspaceSpy = jest
			.spyOn(API, "updateWorkspace")
			.mockRejectedValueOnce(
				new MissingBuildParameters(
					[MockTemplateVersionParameter1, MockTemplateVersionParameter2],
					MockOutdatedWorkspace.template_active_version_id,
				),
			);

		// Render
		await renderWorkspacePage(MockWorkspace);

		// Actions
		const user = userEvent.setup();
		await user.click(screen.getByTestId("workspace-update-button"));
		const confirmButton = await screen.findByTestId("confirm-button");
		await user.click(confirmButton);

		// The update was called
		await waitFor(() => {
			expect(API.updateWorkspace).toBeCalled();
			updateWorkspaceSpy.mockClear();
		});

		// After trying to update, a new dialog asking for missed parameters should
		// be displayed and filled
		const dialog = await screen.findByTestId("dialog");
		const firstParameterInput = within(dialog).getByLabelText(
			MockTemplateVersionParameter1.name,
			{ exact: false },
		);
		await user.clear(firstParameterInput);
		await user.type(firstParameterInput, "some-value");
		const secondParameterInput = within(dialog).getByLabelText(
			MockTemplateVersionParameter2.name,
			{ exact: false },
		);
		await user.clear(secondParameterInput);
		await user.type(secondParameterInput, "2");
		await user.click(
			within(dialog).getByRole("button", { name: /update parameters/i }),
		);

		// Check if the update was called using the values from the form
		await waitFor(() => {
			expect(API.updateWorkspace).toBeCalledWith(MockOutdatedWorkspace, [
				{
					name: MockTemplateVersionParameter1.name,
					value: "some-value",
				},
				{
					name: MockTemplateVersionParameter2.name,
					value: "2",
				},
			]);
		});
	});

	it("restart the workspace with one time parameters when having the confirmation dialog", async () => {
		localStorage.removeItem(`${MockUser.id}_ignoredWarnings`);
		jest.spyOn(API, "getWorkspaceParameters").mockResolvedValue({
			templateVersionRichParameters: [
				{
					...MockTemplateVersionParameter1,
					ephemeral: true,
					name: "rebuild",
					description: "Rebuild",
					required: false,
				},
			],
			buildParameters: [{ name: "rebuild", value: "false" }],
		});
		const restartWorkspaceSpy = jest.spyOn(API, "restartWorkspace");
		const user = userEvent.setup();
		await renderWorkspacePage(MockWorkspace);
		await user.click(screen.getByTestId("build-parameters-button"));
		const buildParametersForm = await screen.findByTestId(
			"build-parameters-form",
		);
		const rebuildField = within(buildParametersForm).getByLabelText("Rebuild", {
			exact: false,
		});
		await user.clear(rebuildField);
		await user.type(rebuildField, "true");
		await user.click(screen.getByTestId("build-parameters-submit"));
		await user.click(screen.getByTestId("confirm-button"));
		await waitFor(() => {
			expect(restartWorkspaceSpy).toBeCalledWith({
				workspace: MockWorkspace,
				buildParameters: [{ name: "rebuild", value: "true" }],
			});
		});
	});

	// Tried to get these wired up via describe.each to reduce repetition, but the
	// syntax just got too convoluted because of the variance in what arguments
	// each function gets called with
	describe("Retrying failed workspaces", () => {
		const retryButtonRe = /^Retry$/i;
		const retryDebugButtonRe = /^Debug$/i;

		describe("Retries a failed 'Start' transition", () => {
			const mockStart = jest.spyOn(API, "startWorkspace");
			const failedStart: Workspace = {
				...MockFailedWorkspace,
				latest_build: {
					...MockFailedWorkspace.latest_build,
					transition: "start",
				},
			};

			test("Retry with no debug", async () => {
				await testButton(failedStart, retryButtonRe, mockStart);

				expect(mockStart).toBeCalledWith(
					failedStart.id,
					failedStart.latest_build.template_version_id,
					undefined,
					undefined,
				);
			});

			test("Retry with debug logs", async () => {
				await testButton(failedStart, retryDebugButtonRe, mockStart);

				expect(mockStart).toBeCalledWith(
					failedStart.id,
					failedStart.latest_build.template_version_id,
					"debug",
					undefined,
				);
			});
		});

		describe("Retries a failed 'Stop' transition", () => {
			const mockStop = jest.spyOn(API, "stopWorkspace");
			const failedStop: Workspace = {
				...MockFailedWorkspace,
				latest_build: {
					...MockFailedWorkspace.latest_build,
					transition: "stop",
				},
			};

			test("Retry with no debug", async () => {
				await testButton(failedStop, retryButtonRe, mockStop);
				expect(mockStop).toBeCalledWith(failedStop.id, undefined);
			});

			test("Retry with debug logs", async () => {
				await testButton(failedStop, retryDebugButtonRe, mockStop);
				expect(mockStop).toBeCalledWith(failedStop.id, "debug");
			});
		});

		describe("Retries a failed 'Delete' transition", () => {
			const mockDelete = jest.spyOn(API, "deleteWorkspace");
			const failedDelete: Workspace = {
				...MockFailedWorkspace,
				latest_build: {
					...MockFailedWorkspace.latest_build,
					transition: "delete",
				},
			};

			test("Retry with no debug", async () => {
				await testButton(failedDelete, retryButtonRe, mockDelete);
				expect(mockDelete).toBeCalledWith(failedDelete.id, {
					logLevel: undefined,
				});
			});

			test("Retry with debug logs", async () => {
				await testButton(failedDelete, retryDebugButtonRe, mockDelete);
				expect(mockDelete).toBeCalledWith<
					[string, apiModule.DeleteWorkspaceOptions]
				>(failedDelete.id, {
					log_level: "debug",
				});
			});
		});
	});

	it("retry with build parameters", async () => {
		const user = userEvent.setup();
		const workspace = {
			...MockFailedWorkspace,
			latest_build: {
				...MockFailedWorkspace.latest_build,
				transition: "start",
			},
		} satisfies Workspace;
		const parameter = {
			...MockTemplateVersionParameter1,
			display_name: "Parameter 1",
			ephemeral: true,
		} satisfies TemplateVersionParameter;

		server.use(
			http.get("/api/v2/templateversions/:versionId/rich-parameters", () => {
				return HttpResponse.json([parameter]);
			}),
		);
		const startWorkspaceSpy = jest.spyOn(API, "startWorkspace");

		await renderWorkspacePage(workspace);
		const retryWithBuildParametersButton = await screen.findByRole("button", {
			name: "Retry with build parameters",
		});
		await user.click(retryWithBuildParametersButton);
		await screen.findByText("Build Options");
		const parameterField = screen.getByLabelText(parameter.display_name, {
			exact: false,
		});
		await user.clear(parameterField);
		await user.type(parameterField, "some-value");
		const submitButton = screen.getByText("Build workspace");
		await user.click(submitButton);

		await waitFor(() => {
			expect(startWorkspaceSpy).toBeCalledWith(
				workspace.id,
				workspace.latest_build.template_version_id,
				undefined,
				[{ name: parameter.name, value: "some-value" }],
			);
		});
	});

	it("debug with build parameters", async () => {
		const user = userEvent.setup();
		const workspace = {
			...MockFailedWorkspace,
			latest_build: {
				...MockFailedWorkspace.latest_build,
				transition: "start",
			},
		} satisfies Workspace;
		const parameter = {
			...MockTemplateVersionParameter1,
			display_name: "Parameter 1",
			ephemeral: true,
		} satisfies TemplateVersionParameter;

		server.use(
			http.get("/api/v2/templateversions/:versionId/rich-parameters", () => {
				return HttpResponse.json([parameter]);
			}),
		);
		const startWorkspaceSpy = jest.spyOn(API, "startWorkspace");

		await renderWorkspacePage(workspace);
		const retryWithBuildParametersButton = await screen.findByRole("button", {
			name: "Debug with build parameters",
		});
		await user.click(retryWithBuildParametersButton);
		await screen.findByText("Build Options");
		const parameterField = screen.getByLabelText(parameter.display_name, {
			exact: false,
		});
		await user.clear(parameterField);
		await user.type(parameterField, "some-value");
		const submitButton = screen.getByText("Build workspace");
		await user.click(submitButton);

		await waitFor(() => {
			expect(startWorkspaceSpy).toBeCalledWith(
				workspace.id,
				workspace.latest_build.template_version_id,
				"debug",
				[{ name: parameter.name, value: "some-value" }],
			);
		});
	});

	describe("Navigation to other pages", () => {
		it("Shows a quota link when quota budget is greater than 0. Link navigates user to /workspaces route with the URL params populated with the corresponding organization", async () => {
			jest.spyOn(API, "getWorkspaceQuota").mockResolvedValueOnce({
				budget: 25,
				credits_consumed: 2,
			});

			const MockDashboardProvider: typeof DashboardProvider = ({
				children,
			}) => (
				<DashboardContext.Provider
					value={{
						appearance: MockAppearanceConfig,
						entitlements: MockEntitlements,
						experiments: [],
						organizations: [MockOrganization],
						showOrganizations: true,
						canViewOrganizationSettings: true,
					}}
				>
					{children}
				</DashboardContext.Provider>
			);

			let destinationLocation!: Location;
			const MockWorkspacesPage: FC = () => {
				destinationLocation = useLocation();
				return null;
			};

			const workspace: Workspace = {
				...MockWorkspace,
				organization_name: MockOrganization.name,
			};

			await renderWorkspacePage(workspace, {
				mockAuthProviders: {
					DashboardProvider: MockDashboardProvider,
				},
				extraRoutes: [
					{
						path: "/workspaces",
						element: <MockWorkspacesPage />,
					},
				],
			});

			const quotaLink = await screen.findByRole<HTMLAnchorElement>("link", {
				name: /\d+ credits of \d+/i,
			});

			const orgName = encodeURIComponent(MockOrganization.name);
			expect(
				quotaLink.href.endsWith(`/workspaces?filter=organization:${orgName}`),
			).toBe(true);

			const user = userEvent.setup();
			await user.click(quotaLink);

			expect(destinationLocation.pathname).toBe("/workspaces");
			expect(destinationLocation.search).toBe(
				`?filter=organization:${orgName}`,
			);
		});
	});
});

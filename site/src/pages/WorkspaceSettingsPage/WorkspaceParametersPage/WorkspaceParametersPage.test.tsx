import { screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockPreviewParameter1,
	MockPreviewParameter2,
	MockPreviewParameter4,
	MockPreviewParameter7,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter4,
	MockTemplateVersionParameter7,
	MockWorkspace,
	MockWorkspaceBuildParameter1,
	MockWorkspaceBuildParameter4,
	MockWorkspaceBuildParameter7,
} from "#/testHelpers/entities";
import {
	checkParameters,
	editParameters,
	isBuildParameter,
} from "#/testHelpers/parameters";
import {
	renderWithWorkspaceSettingsLayout,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "#/testHelpers/websockets";
import WorkspaceParametersPage from "./WorkspaceParametersPage";

describe("WorkspaceParametersPage", () => {
	const renderWorkspaceParametersPage = (
		route = `/@${MockWorkspace.owner_name}/${MockWorkspace.name}/settings`,
	) => {
		return renderWithWorkspaceSettingsLayout(<WorkspaceParametersPage />, {
			route,
			path: "/:username/:workspace/settings",
			extraRoutes: [
				{
					// Need this because after submit the user is redirected.
					path: "/:username/:workspace",
					element: <div>Workspace Page</div>,
				},
			],
		});
	};

	beforeEach(() => {
		vi.clearAllMocks();
		vi.spyOn(API, "getWorkspaceByOwnerAndName").mockResolvedValueOnce(
			MockWorkspace,
		);
		vi.spyOn(API, "getTemplateVersionRichParameters").mockResolvedValueOnce([
			MockTemplateVersionParameter1, // a mutable string
			MockTemplateVersionParameter4, // an immutable string
			MockTemplateVersionParameter7, // optional string
		]);
		vi.spyOn(API, "postWorkspaceBuild").mockRejectedValueOnce(
			new Error("not implemented"),
		);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("waits for and sends initial build parameters", async () => {
		const { promise, resolve } =
			createDeferred<TypesGen.WorkspaceBuildParameter[]>();
		vi.spyOn(API, "getWorkspaceBuildParameters").mockReturnValueOnce(promise);

		const [_, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			// The initial message always has the default values.
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: -1,
						parameters: [
							MockPreviewParameter1,
							MockPreviewParameter4,
							MockPreviewParameter7,
						],
						diagnostics: [],
					}),
				}),
			);
		});

		renderWorkspaceParametersPage();

		// Wait for both requests to have been made.  Client should not have sent
		// any message yet since build parameters have not resolved.
		await waitFor(() => {
			expect(API.getWorkspaceBuildParameters).toHaveBeenCalled();
			expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			expect(mockPublisher.clientSentData).toHaveLength(0);
		});

		// Build parameters now resolve.
		const buildParameters = [
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter4,
			MockWorkspaceBuildParameter7,
		];
		await act(async () => {
			resolve(buildParameters);
		});

		// The client's init message should include all the build values.
		await waitFor(() => {
			expect(mockPublisher.clientSentData).toHaveLength(1);
			expect(JSON.parse(mockPublisher.clientSentData[0] as string)).toEqual(
				expect.objectContaining({
					id: 0,
					inputs: Object.fromEntries(
						buildParameters.map((p) => [p.name, p.value]),
					),
				}),
			);
		});

		// Should still be waiting for the response.
		expect(screen.queryByTestId("loader")).toBeInTheDocument();

		// Respond to the init message with up-to-date values.
		mockPublisher.publishMessage(
			new MessageEvent("message", {
				data: JSON.stringify({
					id: 0,
					parameters: [
						{
							...MockPreviewParameter1,
							value: { valid: true, value: MockWorkspaceBuildParameter1.value },
						},
						{
							...MockPreviewParameter4,
							value: { valid: true, value: MockWorkspaceBuildParameter4.value },
						},
						{
							...MockPreviewParameter7,
							value: { valid: true, value: MockWorkspaceBuildParameter7.value },
						},
					],
					diagnostics: [],
				}),
			}),
		);

		// Finally the page is rendered with the build values.
		await waitForLoaderToBeRemoved();
		await checkParameters(
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter4,
			MockWorkspaceBuildParameter7,
		);

		// The submit button should be enabled.
		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /update and restart/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
	});

	it("skips zero-length initial parameters", async () => {
		const { promise, resolve } =
			createDeferred<TypesGen.WorkspaceBuildParameter[]>();
		vi.spyOn(API, "getWorkspaceBuildParameters").mockReturnValueOnce(promise);

		const [_, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			// The initial message always has the default values.
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: -1,
						parameters: [
							MockPreviewParameter1,
							MockPreviewParameter4,
							MockPreviewParameter7,
						],
						diagnostics: [],
					}),
				}),
			);
		});

		renderWorkspaceParametersPage();

		// Wait for both requests to have been made.  Client should not have sent
		// any message yet since build parameters have not resolved.
		await waitFor(() => {
			expect(API.getWorkspaceBuildParameters).toHaveBeenCalled();
			expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			expect(mockPublisher.clientSentData).toHaveLength(0);
		});

		// Build parameters now resolve.
		await act(async () => {
			resolve([]);
		});

		// Since there are no build values, the page is rendered with defaults and
		// the client does not need to send anything.
		await waitForLoaderToBeRemoved();
		await checkParameters(
			MockPreviewParameter1,
			MockPreviewParameter4,
			MockPreviewParameter7,
		);
		expect(mockPublisher.clientSentData).toHaveLength(0);

		// The submit button should be enabled.
		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /update and restart/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
	});

	it("does not clobber build parameters", async () => {
		const buildParameters = [
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter4,
			MockWorkspaceBuildParameter7,
		];

		vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce(
			buildParameters,
		);

		const [, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			// The initial message always has the default values.
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: -1,
						parameters: [
							MockPreviewParameter1,
							MockPreviewParameter4,
							MockPreviewParameter7,
						],
						diagnostics: [],
					}),
				}),
			);
		});

		renderWorkspaceParametersPage();

		// Wait for the client's init message then respond with different values.
		await waitFor(() => {
			expect(mockPublisher.clientSentData).toHaveLength(1);
			expect(JSON.parse(mockPublisher.clientSentData[0] as string)).toEqual(
				expect.objectContaining({
					id: 0,
					inputs: Object.fromEntries(
						buildParameters.map((p) => [p.name, p.value]),
					),
				}),
			);
		});

		mockPublisher.publishMessage(
			new MessageEvent("message", {
				data: JSON.stringify({
					id: 0,
					parameters: [
						MockPreviewParameter1,
						MockPreviewParameter2, // new field
						MockPreviewParameter4,
						MockPreviewParameter7,
					],
					diagnostics: [],
				}),
			}),
		);

		// Page should render with the build values, but the new field that was not
		// part of the previous build should also show up.
		await waitForLoaderToBeRemoved();
		await checkParameters(
			MockWorkspaceBuildParameter1,
			MockWorkspaceBuildParameter4,
			MockWorkspaceBuildParameter7,
			MockPreviewParameter2,
		);

		// However the submit button should be disabled because the state
		// mismatches.
		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /update and restart/i,
		});
		await waitFor(() => expect(submitButton).toBeDisabled());
	});

	it("does not clobber edited parameters", async () => {
		vi.spyOn(API, "getWorkspaceBuildParameters").mockResolvedValueOnce([]);

		const [, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			// The initial message always has the default values.
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: -1,
						parameters: [
							MockPreviewParameter1,
							MockPreviewParameter4,
							MockPreviewParameter7,
						],
						diagnostics: [],
					}),
				}),
			);
		});

		renderWorkspaceParametersPage();

		// Page should render with the default values.
		await waitForLoaderToBeRemoved();
		await checkParameters(
			MockPreviewParameter1,
			MockPreviewParameter4,
			MockPreviewParameter7,
		);

		// Blank out one field and fill out another.
		const editedParameters = [
			// Put the blank one first to ensure we are preserving blank values and
			// not just including it the first time due to the change handler.
			{
				name: MockPreviewParameter1.name,
				value: "",
			},
			{
				name: MockPreviewParameter7.name,
				value: "not-blank",
			},
		];
		editParameters(...editedParameters);

		// The client should now send all parameters.
		await waitFor(() => {
			expect(mockPublisher.clientSentData).toHaveLength(1);
			expect(JSON.parse(mockPublisher.clientSentData[0] as string)).toEqual(
				expect.objectContaining({
					id: 0,
					inputs: Object.fromEntries(
						[...editedParameters, MockPreviewParameter4].map((p) => [
							p.name,
							isBuildParameter(p) ? p.value : p.value.value,
						]),
					),
				}),
			);
		});

		// Respond with different values.
		mockPublisher.publishMessage(
			new MessageEvent("message", {
				data: JSON.stringify({
					id: 0,
					parameters: [
						MockPreviewParameter1,
						MockPreviewParameter2, // new field
						MockPreviewParameter4,
						MockPreviewParameter7,
					],
					diagnostics: [],
				}),
			}),
		);

		// The form should keep the user's values but include the new field.
		await checkParameters(
			...editedParameters,
			MockPreviewParameter4,
			MockPreviewParameter2,
		);

		// However the submit button should be disabled because the state
		// mismatches.
		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /update and restart/i,
		});
		await waitFor(() => expect(submitButton).toBeDisabled());
	});
});

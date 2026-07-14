import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { API } from "#/api/api";
import type { DynamicParametersResponse, Preset } from "#/api/typesGenerated";
import {
	MockDropdownParameter,
	MockDynamicParametersResponse,
	MockDynamicParametersResponseWithError,
	MockPermissions,
	MockPreviewParameter1,
	MockPreviewParameter2,
	MockPreviewParameter7,
	MockSliderParameter,
	MockTemplate,
	MockTemplateVersion,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	MockValidationParameter,
	MockWorkspace,
} from "#/testHelpers/entities";
import { checkParameters, editParameters } from "#/testHelpers/parameters";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "#/testHelpers/renderHelpers";
import { mockDynamicParameterWebSocket } from "#/testHelpers/websockets";
import CreateWorkspacePage from "./CreateWorkspacePage";

describe("CreateWorkspacePage", () => {
	const renderCreateWorkspacePage = (
		route = `/templates/${MockTemplate.name}/workspace`,
	) => {
		return renderWithAuth(<CreateWorkspacePage />, {
			route,
			path: "/templates/:template/workspace",
			extraRoutes: [
				{
					path: "/:username/:workspace",
					element: <div>Workspace Page</div>,
				},
			],
		});
	};

	const renderCreateWorkspacePageWithSocket = (route?: string) => {
		mockDynamicParameterWebSocket((publisher) => {
			publisher.publishOpen(new Event("open"));
			publisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify(MockDynamicParametersResponse),
				}),
			);
		});

		return renderCreateWorkspacePage(route);
	};

	const mockGpuPreset: Preset = {
		ID: "preset-gpu",
		Name: "gpu-large",
		Parameters: [
			{ Name: "instance_type", Value: "t3.medium" },
			{ Name: "cpu_count", Value: "4" },
		],
		Default: false,
		DesiredPrebuildInstances: null,
		Description: "GPU Large preset",
		Icon: "",
	};

	beforeEach(() => {
		vi.clearAllMocks();

		vi.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		vi.spyOn(API, "getTemplateVersion").mockResolvedValue(MockTemplateVersion);
		vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		vi.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		vi.spyOn(API, "checkAuthorization").mockResolvedValue(MockPermissions);
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	describe("WebSocket Integration", () => {
		it("establishes WebSocket connection and receives initial parameters", async () => {
			renderCreateWorkspacePageWithSocket();
			await waitForLoaderToBeRemoved();

			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
				MockTemplate.active_version_id,
				MockUserOwner.id,
				expect.objectContaining({
					onMessage: expect.any(Function),
					onError: expect.any(Function),
					onClose: expect.any(Function),
				}),
			);

			await waitFor(() => {
				expect(screen.getByText(/instance type/i)).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			const [mockWebSocket] = mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify(MockDynamicParametersResponse),
					}),
				);
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const instanceTypeField = screen.getByTestId(
				"parameter-field-instance_type",
			);
			const instanceTypeSelect =
				within(instanceTypeField).getByRole("combobox");
			expect(instanceTypeSelect).toBeInTheDocument();

			vi.useFakeTimers({ shouldAdvanceTime: true });

			await userEvent.click(instanceTypeSelect);

			const mediumOption = await screen.findByRole("option", {
				name: /t3\.medium/i,
			});

			await userEvent.click(mediumOption);

			await act(async () => {
				await vi.runAllTimersAsync();
			});

			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"instance_type":"t3.medium"'),
			);

			vi.useRealTimers();
		});

		it("handles WebSocket error gracefully", async () => {
			const [_, mockPublisher] = mockDynamicParameterWebSocket();

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			});

			await act(async () => {
				mockPublisher.publishError(new Event("Connection failed"));
			});

			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /connection for dynamic parameters failed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			const [_, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: -1,
							parameters: [],
							diagnostics: [],
						}),
					}),
				);
			});

			renderCreateWorkspacePage();

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			});

			await act(async () => {
				mockPublisher.publishClose(new Event("close") as CloseEvent);
			});

			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /websocket connection.*unexpectedly closed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("only parameters from latest response are displayed", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: -1,
							parameters: [MockDropdownParameter],
							diagnostics: [],
						}),
					}),
				);
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			const response1: DynamicParametersResponse = {
				id: 1,
				parameters: [MockDropdownParameter],
				diagnostics: [],
			};
			const response2: DynamicParametersResponse = {
				id: 4,
				parameters: [MockSliderParameter],
				diagnostics: [],
			};

			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response1) }),
				);

				mockPublisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response2) }),
				);
			});

			await waitFor(() => {
				expect(screen.queryByText("CPU Count")).toBeInTheDocument();
				expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
			});
		});

		it("does not clobber user values", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				// The initial message always has the default values.
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: -1,
							parameters: [MockPreviewParameter1, MockPreviewParameter7],
							diagnostics: [],
						}),
					}),
				);
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

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

			// Respond with different values.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters: [
								MockPreviewParameter1,
								MockPreviewParameter2, // new field
								MockPreviewParameter7,
							],
							diagnostics: [],
						}),
					}),
				);
			});

			// Should have the new field, but keep the existing user-filled values.
			await checkParameters(...editedParameters, MockPreviewParameter2);
		});

		it("does not clobber auto-filled values", async () => {
			const [, mockPublisher] = mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				// The initial message always has the default values.
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: -1,
							parameters: [MockPreviewParameter1, MockPreviewParameter7],
							diagnostics: [],
						}),
					}),
				);
			});

			// Blank out one field and fill out another.
			const editedParameters = [
				{
					name: MockPreviewParameter1.name,
					value: "",
				},
				{
					name: MockPreviewParameter7.name,
					value: "not-blank",
				},
			];
			const query = editedParameters
				.map((param) => `param.${param.name}=${param.value}`)
				.join("&");
			renderCreateWorkspacePage(
				`/templates/${MockTemplate.name}/workspace?${query}`,
			);
			await waitForLoaderToBeRemoved();

			// Respond with different values.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters: [
								MockPreviewParameter1,
								MockPreviewParameter2, // new field
								MockPreviewParameter7,
							],
							diagnostics: [],
						}),
					}),
				);
			});

			// Should have the new field, but keep the existing auto-filled values.
			await checkParameters(...editedParameters, MockPreviewParameter2);
		});
	});

	describe("Dynamic Parameter Types", () => {
		it("displays parameter validation errors", async () => {
			mockDynamicParameterWebSocket((publisher) => {
				publisher.publishOpen(new Event("open"));
				publisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify(MockDynamicParametersResponseWithError),
					}),
				);
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Validation failed")).toBeInTheDocument();
				expect(
					screen.getByText(
						"The selected instance type is not available in this region",
					),
				).toBeInTheDocument();
			});
		});

		it("displays parameter validation errors for min/max constraints", async () => {
			const mockResponseInitial: DynamicParametersResponse = {
				id: 1,
				parameters: [MockValidationParameter],
				diagnostics: [],
			};

			const mockResponseWithError: DynamicParametersResponse = {
				id: 2,
				parameters: [
					{
						...MockValidationParameter,
						value: { value: "200", valid: false },
						diagnostics: [
							{
								severity: "error",
								summary:
									"Invalid parameter value according to 'validation' block",
								detail: "value 200 is more than the maximum 100",
								extra: {
									code: "",
								},
							},
						],
					},
				],
				diagnostics: [],
			};

			const [mockWebSocket, mockPublisher] = mockDynamicParameterWebSocket(
				(publisher) => {
					publisher.publishOpen(new Event("open"));
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockResponseInitial),
						}),
					);
				},
			);
			const originalSend = mockWebSocket.send;
			mockWebSocket.send = vi.fn((data) => {
				originalSend.call(mockWebSocket, data);

				if (typeof data === "string" && data.includes('"200"')) {
					mockPublisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockResponseWithError),
						}),
					);
				}
			});

			renderCreateWorkspacePage();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Invalid Parameter")).toBeInTheDocument();
			});

			const numberInput = screen.getByDisplayValue("50");
			expect(numberInput).toBeInTheDocument();

			await userEvent.clear(numberInput);
			await userEvent.type(numberInput, "200");

			await waitFor(() => {
				expect(screen.getByDisplayValue("200")).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText(
						"Invalid parameter value according to 'validation' block",
					),
				).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText("value 200 is more than the maximum 100"),
				).toBeInTheDocument();
			});

			const errorElement = screen.getByText(
				"value 200 is more than the maximum 100",
			);
			expect(errorElement.closest("div")).toHaveClass(
				"text-content-destructive",
			);
		});
	});

	describe("External Authentication", () => {
		it("displays external auth providers", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithub,
			]);

			renderCreateWorkspacePageWithSocket();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /login with github/i }),
				).toBeInTheDocument();
			});
		});

		it("shows authenticated state for connected providers", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);

			renderCreateWorkspacePageWithSocket();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithub,
			]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?mode=auto&version=${MockTemplate.id}`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(
					screen.getByText(
						/external authentication provider that is not connected/i,
					),
				).toBeInTheDocument();
				expect(
					screen.getByText(/auto-creation has been disabled/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("Auto-creation Mode", () => {
		it("falls back to form mode when auto-creation fails", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "createWorkspace").mockRejectedValue(
				new Error("Auto-creation failed"),
			);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);

			// Consent dialog appears for mode=auto. Confirm to proceed.
			const confirmButton = await screen.findByRole("button", {
				name: /confirm and create/i,
			});
			await userEvent.click(confirmButton);

			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			await waitFor(() => {
				expect(screen.getByText("Create workspace")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /create workspace/i }),
				).toBeInTheDocument();
			});
		});
	});

	describe("Form Submission", () => {
		it("creates workspace with correct parameters", async () => {
			renderCreateWorkspacePageWithSocket();
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});
			await userEvent.clear(nameInput);
			await userEvent.type(nameInput, "my-test-workspace");

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					"test-user",
					expect.objectContaining({
						name: "my-test-workspace",
						template_version_id: MockTemplate.active_version_id,
						template_id: undefined,
						rich_parameter_values: [
							expect.objectContaining({ name: "instance_type", value: "" }),
							expect.objectContaining({ name: "cpu_count", value: "2" }),
							expect.objectContaining({
								name: "enable_monitoring",
								value: "true",
							}),
							expect.objectContaining({ name: "tags", value: "[]" }),
							expect.objectContaining({ name: "ides", value: "[]" }),
						],
					}),
				);
			});
		});
	});

	describe("URL Parameters", () => {
		it("pre-fills parameters from URL", async () => {
			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`,
			);
			await waitForLoaderToBeRemoved();

			expect(screen.getByText(/instance type/i)).toBeInTheDocument();
			expect(screen.getByText("CPU Count")).toBeInTheDocument();
		});

		it("uses custom template version when specified", async () => {
			const customVersionId = "custom-version-123";

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`,
			);

			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
					customVersionId,
					MockUserOwner.id,
					expect.any(Object),
				);
			});
		});

		it("pre-fills workspace name from URL", async () => {
			const workspaceName = "my-custom-workspace";

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				const nameInput = screen.getByRole("textbox", {
					name: /workspace name/i,
				});
				expect(nameInput).toHaveValue(workspaceName);
			});
		});
	});

	describe("URL Presets", () => {
		it("resolves a preset from the URL and selects it in the form", async () => {
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				mockGpuPreset,
			]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?preset=gpu-large`,
			);
			await waitForLoaderToBeRemoved();

			expect(
				screen.getByRole("button", { name: /gpu-large/i }),
			).toBeInTheDocument();
		});

		it("resolves a preset against the pinned template version", async () => {
			const getTemplateVersionPresetsSpy = vi
				.spyOn(API, "getTemplateVersionPresets")
				.mockResolvedValue([mockGpuPreset]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?version=custom-version&preset=gpu-large`,
			);

			await waitFor(() => {
				expect(getTemplateVersionPresetsSpy).toHaveBeenCalledWith(
					"custom-version",
				);
			});
		});

		it("falls back to form mode when auto-create cannot resolve the preset", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				mockGpuPreset,
			]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?mode=auto&preset=missing`,
			);
			await waitForLoaderToBeRemoved();

			expect(
				screen.queryByRole("button", { name: /confirm and create/i }),
			).not.toBeInTheDocument();
			expect(
				screen.getByText(/auto-creation has been disabled/i),
			).toBeInTheDocument();
			expect(
				screen.getByText(
					/preset "missing" not found on template version "test-version"/i,
				),
			).toBeInTheDocument();
			expect(API.createWorkspace).not.toHaveBeenCalled();
		});

		it("falls back to form mode when presets fail to load", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "getTemplateVersionPresets").mockRejectedValue(
				new Error("presets unavailable"),
			);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?mode=auto&preset=gpu-large`,
			);
			await waitForLoaderToBeRemoved();

			expect(
				screen.queryByRole("button", { name: /confirm and create/i }),
			).not.toBeInTheDocument();
			expect(
				screen.getByText(/auto-creation has been disabled/i),
			).toBeInTheDocument();
			expect(
				screen.getByText(/failed to load presets: presets unavailable/i),
			).toBeInTheDocument();
			expect(API.createWorkspace).not.toHaveBeenCalled();
		});

		it("uses preset parameters instead of param values", async () => {
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				mockGpuPreset,
			]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?preset=gpu-large&param.instance_type=t3.small&param.cpu_count=99`,
			);
			await waitForLoaderToBeRemoved();

			expect(screen.getAllByText(/param\.\*/i).length).toBeGreaterThan(0);

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});
			await userEvent.type(nameInput, "preset-workspace");

			await userEvent.click(
				screen.getByRole("button", { name: /create workspace/i }),
			);

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					"test-user",
					expect.objectContaining({
						template_version_preset_id: mockGpuPreset.ID,
						rich_parameter_values: expect.arrayContaining([
							expect.objectContaining({
								name: "instance_type",
								value: "t3.medium",
							}),
							expect.objectContaining({ name: "cpu_count", value: "4" }),
						]),
					}),
				);
			});
		});

		it("auto-creates with the preset ID after the preset resolves", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				mockGpuPreset,
			]);

			renderCreateWorkspacePageWithSocket(
				`/templates/${MockTemplate.name}/workspace?mode=auto&preset=gpu-large&name=preset-workspace`,
			);

			const confirmButton = await screen.findByRole("button", {
				name: /confirm and create/i,
			});
			await userEvent.click(confirmButton);

			await waitFor(() => {
				expect(API.createWorkspace).toHaveBeenCalledWith(
					"me",
					expect.objectContaining({
						name: "preset-workspace",
						template_version_preset_id: mockGpuPreset.ID,
						rich_parameter_values: [],
					}),
				);
			});
		});
	});

	describe("Navigation", () => {
		it("navigates to workspace after successful creation", async () => {
			const { router } = renderCreateWorkspacePageWithSocket();
			await waitForLoaderToBeRemoved();

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});

			await userEvent.clear(nameInput);
			await userEvent.type(nameInput, "my-test-workspace");

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await userEvent.click(createButton);

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(
					`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
				);
			});
		});
	});
});

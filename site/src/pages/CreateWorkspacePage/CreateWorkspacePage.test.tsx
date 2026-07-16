import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { API } from "#/api/api";
import type { Preset, PreviewParameter } from "#/api/typesGenerated";
import {
	MockDropdownParameter,
	MockDynamicParametersResponseWithError,
	MockMultiSelectParameter,
	MockPermissions,
	MockPreviewParameter1,
	MockPreviewParameter2,
	MockPreviewParameter7,
	MockSliderParameter,
	MockSwitchParameter,
	MockTagSelectParameter,
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
import {
	type MockWebSocket,
	type MockWebSocketServer,
	mockDynamicParameterWebSocket,
} from "#/testHelpers/websockets";
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

	type Context = ReturnType<typeof renderCreateWorkspacePage> & {
		mockSocket: MockWebSocket;
		mockPublisher: MockWebSocketServer;
	};

	// Mocks the required endpoints, most importantly the web socket, constructs
	// the route with the required query parameters, then renders the page on that
	// route.
	//
	// Returns the mock web socket and router context for further testing.
	const renderPageWithSocket = async (
		opts: {
			// route can be overridden to set additional query variables.
			route?: string;
			// urlfill contains template parameters auto-filled via the query string.
			urlfill?: Record<string, string>;
			// version will be added to the query string.
			version?: string;
			// preset will be added to the query string and set on the preset endpoint.
			preset?: Preset;
		} = {},
	): Promise<Context> => {
		const [mockSocket, mockPublisher] = mockDynamicParameterWebSocket();

		const params = new URLSearchParams();
		if (opts.urlfill) {
			Object.entries(opts.urlfill).forEach(([k, v]) => {
				params.set(`param.${k}`, v);
			});
		}
		if (opts.preset) {
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				opts.preset,
			]);
			params.set("preset", opts.preset.Name);
		}
		if (opts.version) {
			params.set("version", opts.version);
		}
		let route = opts?.route || `/templates/${MockTemplate.name}/workspace`;
		const query = params.toString();
		if (query.length > 0) {
			if (route.includes("?")) {
				route += `&${query}`;
			} else {
				route += `?${query}`;
			}
		}
		const router = renderCreateWorkspacePage(route);
		return { mockSocket, mockPublisher, ...router };
	};

	// Waits for the client to connect to the socket then sends the initial
	// message.  Then, if there are auto-fill parameters (whether from a URL or a
	// preset), also waits for the client's initial message.
	const expectSocketHandshake = async (opts: {
		mockPublisher: MockWebSocketServer;
		// parameters are template parameters to send via the initial message from
		// the backend to the client.
		parameters: PreviewParameter[];
		// urlfill will be expected in the client's init message.
		urlfill?: Record<string, string>;
		// version will be asserted in the web socket API call.
		version?: string;
		// preset will be expected in the client's init message.
		preset?: Preset;
	}): Promise<void> => {
		// Wait for the web socket connection.
		const version = opts.version || MockTemplate.active_version_id;
		await waitFor(() => {
			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
				version,
				MockUserOwner.id,
				expect.objectContaining({
					onMessage: expect.any(Function),
					onError: expect.any(Function),
					onClose: expect.any(Function),
				}),
			);
		});

		// Open and and send the initial message.
		await act(async () => {
			opts.mockPublisher.publishOpen(new Event("open"));
			// The initial message always has the default values.
			opts.mockPublisher.publishMessage(
				new MessageEvent("message", {
					data: JSON.stringify({
						id: -1,
						parameters: opts.parameters,
						diagnostics: [],
					}),
				}),
			);
		});

		// Wait for the client's own init message, which should include all the
		// auto-filled values, including from a preset.  Without any auto-fill
		// values, the client does not send any init message.
		const inputs = opts.urlfill ? { ...opts.urlfill } : {};
		opts.preset?.Parameters?.forEach((p) => {
			inputs[p.Name] = p.Value;
		});
		if (Object.keys(inputs).length > 0) {
			await waitFor(() => {
				expect(opts.mockPublisher.clientSentData).toHaveLength(1);
				expect(
					JSON.parse(opts.mockPublisher.clientSentData[0] as string),
				).toEqual(
					expect.objectContaining({
						id: 0,
						inputs,
					}),
				);
			});
		}
	};

	// Wait for the loader to be removed then asserts form fields based on
	// parameters and auto-fill.  Lastly asserts the submit button is enabled.
	const expectFormFields = async (opts: {
		parameters: PreviewParameter[];
		urlfill?: Record<string, string>;
		preset?: Preset;
	}): Promise<void> => {
		// Add any preset to the autofill.
		const autofill = opts.urlfill ? { ...opts.urlfill } : {};
		opts.preset?.Parameters?.forEach((p) => {
			autofill[p.Name] = p.Value;
		});

		await waitForLoaderToBeRemoved();

		const parameters = opts.parameters.map((p) => {
			return {
				...p,
				value: Object.hasOwn(autofill, p.name)
					? { valid: true, value: autofill[p.name] }
					: p.value,
			};
		});

		// The page should render with the defaults plus any auto-fill.
		await checkParameters(...parameters);
		const form = screen.getByTestId("form");
		const submitButton = within(form).getByRole("button", {
			name: /create workspace/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
	};

	const mockGpuPreset: Preset = {
		ID: "preset-gpu",
		Name: "gpu-large",
		Parameters: [
			{ Name: MockDropdownParameter.name, Value: "t3.medium" },
			{ Name: MockSliderParameter.name, Value: "4" },
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
		it("skips initial parameters when no auto-fill", async () => {
			const parameters = [
				MockDropdownParameter,
				MockSliderParameter,
				MockSwitchParameter,
				MockTagSelectParameter,
				MockMultiSelectParameter,
			];
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters });
			// Should render without any sending any init message.
			await expectFormFields({ parameters });
			expect(mockPublisher.clientSentData).toHaveLength(0);
		});

		it("waits for and sends initial parameters when auto-filled", async () => {
			const parameters = [
				MockDropdownParameter,
				MockSliderParameter,
				MockSwitchParameter,
				MockTagSelectParameter,
				MockMultiSelectParameter,
			];
			const urlfill = {
				[MockDropdownParameter.name]: "t3.micro",
				[MockSliderParameter.name]: "55",
				[MockSwitchParameter.name]: "false",
				[MockTagSelectParameter.name]: JSON.stringify(["tag1", "tag2"]),
				[MockMultiSelectParameter.name]: JSON.stringify(["goland", "vscode"]),
			};
			const { mockPublisher } = await renderPageWithSocket({ urlfill });
			await expectSocketHandshake({ mockPublisher, parameters, urlfill });
			// Should still see the loader as the client wais for the response to the
			// client's init message.
			expect(screen.queryByTestId("loader")).toBeInTheDocument();
		});

		it("handles error gracefully", async () => {
			const [_, mockPublisher] = mockDynamicParameterWebSocket();
			renderCreateWorkspacePage();

			// Wait for the client to open the web socket.
			await waitFor(() => {
				expect(API.templateVersionDynamicParameters).toHaveBeenCalled();
			});

			// Then error the web socket.
			await act(async () => {
				mockPublisher.publishError(new Event("Connection failed"));
			});

			// We should see an error message.
			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /connection for dynamic parameters failed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("handles close", async () => {
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters: [] });

			await waitForLoaderToBeRemoved();

			// Close the web socket.
			await act(async () => {
				mockPublisher.publishClose(new Event("close") as CloseEvent);
			});

			// We should see an error message.
			await waitFor(() => {
				const alert = screen.getByRole("alert");
				expect(
					within(alert).getByRole("heading", {
						name: /websocket connection.*unexpectedly closed/i,
					}),
				).toBeInTheDocument();
			});
		});

		it("displays no parameters if none from init message", async () => {
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters: [] });
			await expectFormFields({ parameters: [] });
		});

		it("only parameters from the latest response are displayed", async () => {
			const parameters = [MockDropdownParameter];
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters });
			await expectFormFields({ parameters });

			// Send multiple messages.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 0,
							parameters: [MockSliderParameter],
							diagnostics: [],
						}),
					}),
				);
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: -1,
							parameters: [MockSwitchParameter],
							diagnostics: [],
						}),
					}),
				);
			});

			// Page should re-render with the last message only.
			await checkParameters(MockSwitchParameter);

			// The submit button should still be enabled.
			const form = screen.getByTestId("form");
			const submitButton = within(form).getByRole("button", {
				name: /create workspace/i,
			});
			await waitFor(() => expect(submitButton).toBeEnabled());
		});

		it("does not clobber edited parameters", async () => {
			const parameters = [MockPreviewParameter1, MockPreviewParameter7];
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters });
			await expectFormFields({ parameters });

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

			// Send a message with different values.
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

			// The submit button should still be enabled.
			const form = screen.getByTestId("form");
			const submitButton = within(form).getByRole("button", {
				name: /create workspace/i,
			});
			await waitFor(() => expect(submitButton).toBeEnabled());
		});

		it("does not clobber auto-filled values", async () => {
			const parameters = [MockPreviewParameter1, MockPreviewParameter7];
			// Blank out one field and fill out another.
			const urlfill = {
				[MockPreviewParameter1.name]: "",
				[MockPreviewParameter7.name]: "not-blank",
			};
			const { mockPublisher } = await renderPageWithSocket({ urlfill });
			await expectSocketHandshake({ mockPublisher, parameters, urlfill });

			// Respond to the client's init message with different values.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters: [
								...parameters,
								MockPreviewParameter2, // new field
							],
							diagnostics: [],
						}),
					}),
				);
			});

			await expectFormFields({
				parameters: [...parameters, MockPreviewParameter2],
				urlfill,
			});
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
			const { mockSocket, mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({
				mockPublisher,
				parameters: [MockValidationParameter],
			});

			// Respond to the client's edit with an error.
			mockSocket.send.mockImplementation((data) => {
				expect(JSON.parse(data as string)).toEqual(
					expect.objectContaining({
						id: 0,
						inputs: {
							[MockValidationParameter.name]: "200",
						},
					}),
				);
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
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
						}),
					}),
				);
			});

			const edited = {
				name: MockValidationParameter.name,
				display_name: MockValidationParameter.display_name,
				value: "200",
			};
			editParameters(edited);

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

			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters: [] });

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

			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters: [] });

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithub,
			]);

			const version = MockTemplate.id;
			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?mode=auto`,
				version,
			});
			await expectSocketHandshake({ mockPublisher, parameters: [], version });

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

			const parameters = [
				MockDropdownParameter,
				MockSliderParameter,
				MockSwitchParameter,
				MockTagSelectParameter,
				MockMultiSelectParameter,
			];
			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?mode=auto`,
			});
			await expectSocketHandshake({ mockPublisher, parameters });

			// Consent dialog appears for mode=auto. Confirm to proceed.
			await act(async () => {
				const confirmButton = await screen.findByRole("button", {
					name: /confirm and create/i,
				});
				await userEvent.click(confirmButton);
			});

			await expectFormFields({ parameters });

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
			const parameters = [
				MockDropdownParameter,
				MockSliderParameter,
				MockSwitchParameter,
				MockTagSelectParameter,
				MockMultiSelectParameter,
			];
			const { mockPublisher } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters });
			await expectFormFields({ parameters });

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
						rich_parameter_values: parameters.map((p) => ({
							name: p.name,
							value: p.value.value,
						})),
					}),
				);
			});
		});
	});

	describe("URL Parameters", () => {
		it("uses custom template version when specified", async () => {
			const version = "custom-version-123";
			const { mockPublisher } = await renderPageWithSocket({ version });
			await expectSocketHandshake({ mockPublisher, parameters: [], version });
		});

		it("pre-fills workspace name from URL", async () => {
			const workspaceName = "my-custom-workspace";

			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
			});
			await expectSocketHandshake({ mockPublisher, parameters: [] });

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
		const parameters = [MockDropdownParameter];
		it("resolves a preset from the URL and selects it in the form", async () => {
			const { mockPublisher } = await renderPageWithSocket({
				preset: mockGpuPreset,
			});
			await expectSocketHandshake({ mockPublisher, parameters });

			// Respond to the client's init message.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters,
							diagnostics: [],
						}),
					}),
				);
			});

			await waitForLoaderToBeRemoved();

			expect(
				screen.getByRole("button", { name: /gpu-large/i }),
			).toBeInTheDocument();
		});

		it("resolves a preset against the pinned template version", async () => {
			const version = "custom-version";
			const { mockPublisher } = await renderPageWithSocket({
				version,
				preset: mockGpuPreset,
			});
			await expectSocketHandshake({ mockPublisher, parameters, version });
		});

		it("falls back to form mode when auto-create cannot resolve the preset", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);
			vi.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([
				mockGpuPreset,
			]);

			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?mode=auto&preset=missing`,
			});
			await expectSocketHandshake({ mockPublisher, parameters: [] });

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

			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?mode=auto&preset=gpu-large`,
			});
			await expectSocketHandshake({ mockPublisher, parameters: [] });

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
			const parameters = [MockDropdownParameter, MockSliderParameter];
			const urlfill = {
				[MockDropdownParameter.name]: "t3.small",
				[MockSliderParameter.name]: "99",
			};
			const { mockPublisher } = await renderPageWithSocket({
				preset: mockGpuPreset,
				// Will be overridden by the preset values.
				urlfill,
			});
			await expectSocketHandshake({
				mockPublisher,
				parameters,
				urlfill,
				preset: mockGpuPreset,
			});

			// Respond to the client's init message.  Even though this uses the
			// default values, it should not clobber the preset values.
			await act(async () => {
				mockPublisher.publishMessage(
					new MessageEvent("message", {
						data: JSON.stringify({
							id: 2,
							parameters,
							diagnostics: [],
						}),
					}),
				);
			});

			// No parameters show since they are under the preset section toggle.
			await expectFormFields({ parameters: [] });

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
						rich_parameter_values: mockGpuPreset.Parameters.map((p) => ({
							name: p.Name,
							value: p.Value,
						})),
					}),
				);
			});
		});

		it("auto-creates with the preset ID after the preset resolves", async () => {
			vi.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([
				MockTemplateVersionExternalAuthGithubAuthenticated,
			]);

			const { mockPublisher } = await renderPageWithSocket({
				route: `/templates/${MockTemplate.name}/workspace?mode=auto&name=preset-workspace`,
				preset: mockGpuPreset,
			});
			await expectSocketHandshake({
				mockPublisher,
				parameters: [],
				preset: mockGpuPreset,
			});

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
			const { mockPublisher, router } = await renderPageWithSocket();
			await expectSocketHandshake({ mockPublisher, parameters: [] });

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

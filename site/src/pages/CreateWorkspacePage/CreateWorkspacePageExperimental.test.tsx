import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "api/api";
import type {
	DynamicParametersResponse,
} from "api/typesGenerated";
import {
	MockTemplate,
	MockTemplateVersionExternalAuthGithub,
	MockTemplateVersionExternalAuthGithubAuthenticated,
	MockUserOwner,
	MockWorkspace,
	mockDropdownParameter,
	mockTagSelectParameter,
	mockSwitchParameter,
	mockSliderParameter,
	validationParameter,
} from "testHelpers/entities";
import {
	renderWithAuth,
	waitForLoaderToBeRemoved,
} from "testHelpers/renderHelpers";
import CreateWorkspacePageExperimental from "./CreateWorkspacePageExperimental";

beforeAll(() => {
	if (!Element.prototype.hasPointerCapture) {
		Element.prototype.hasPointerCapture = () => false;
	}
	if (!Element.prototype.setPointerCapture) {
		Element.prototype.setPointerCapture = () => {};
	}
	if (!Element.prototype.releasePointerCapture) {
		Element.prototype.releasePointerCapture = () => {};
	}
});

type MockPublisher = Readonly<{
	publishMessage: (event: MessageEvent<string>) => void;
	publishError: (event: ErrorEvent) => void;
	publishClose: (event: CloseEvent) => void;
	publishOpen: (event: Event) => void;
}>;

type MockWebSocket = Omit<WebSocket, "readyState"> & {
	readyState: number;
};

function createMockWebSocket(
	url: string,
	protocols?: string | string[],
): readonly [WebSocket, MockPublisher] {
	type CallbackStore = {
		[K in keyof WebSocketEventMap]: ((event: WebSocketEventMap[K]) => void)[];
	};

	let activeProtocol: string;
	if (Array.isArray(protocols)) {
		activeProtocol = protocols[0] ?? "";
	} else if (typeof protocols === "string") {
		activeProtocol = protocols;
	} else {
		activeProtocol = "";
	}

	let closed = false;
	const store: CallbackStore = {
		message: [],
		error: [],
		close: [],
		open: [],
	};

	const mockSocket: MockWebSocket = {
		CONNECTING: 0,
		OPEN: 1,
		CLOSING: 2,
		CLOSED: 3,

		url,
		protocol: activeProtocol,
		readyState: 1,
		binaryType: "blob",
		bufferedAmount: 0,
		extensions: "",
		onclose: null,
		onerror: null,
		onmessage: null,
		onopen: null,
		send: jest.fn(),
		dispatchEvent: jest.fn(),

		addEventListener: <E extends keyof WebSocketEventMap>(
			eventType: E,
			callback: WebSocketEventMap[E],
		) => {
			if (closed) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (!subscribers.includes(cb)) {
				subscribers.push(cb);
			}
		},

		removeEventListener: <E extends keyof WebSocketEventMap>(
			eventType: E,
			callback: WebSocketEventMap[E],
		) => {
			if (closed) {
				return;
			}

			const subscribers = store[eventType];
			const cb = callback as unknown as CallbackStore[E][0];
			if (subscribers.includes(cb)) {
				const updated = store[eventType].filter((c) => c !== cb);
				store[eventType] = updated as unknown as CallbackStore[E];
			}
		},

		close: () => {
			if (!closed) {
				closed = true;
				publisher.publishClose(new CloseEvent("close"));
			}
		},
	};

	const publisher: MockPublisher = {
		publishOpen: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.open) {
				sub(event);
			}
			mockSocket.onopen?.(event);
		},

		publishError: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.error) {
				sub(event);
			}
			mockSocket.onerror?.(event);
		},

		publishMessage: (event) => {
			if (closed) {
				return;
			}
			for (const sub of store.message) {
				sub(event);
			}
			mockSocket.onmessage?.(event);
		},

		publishClose: (event) => {
			if (closed) {
				return;
			}
			mockSocket.readyState = 3; // CLOSED
			for (const sub of store.close) {
				sub(event);
			}
			mockSocket.onclose?.(event);
		},
	};

	return [mockSocket, publisher] as const;
}



const mockDynamicParametersResponse: DynamicParametersResponse = {
	id: 1,
	parameters: [
		mockDropdownParameter,
		mockSliderParameter,
		mockSwitchParameter,
		mockTagSelectParameter,
	],
	diagnostics: [],
};

const mockDynamicParametersResponseWithError: DynamicParametersResponse = {
	id: 2,
	parameters: [mockDropdownParameter],
	diagnostics: [
		{
			severity: "error",
			summary: "Validation failed",
			detail: "The selected instance type is not available in this region",
			extra: {
				code: "",
			},
		},
	],
};

const renderCreateWorkspacePageExperimental = (
	route = `/templates/${MockTemplate.name}/workspace`,
) => {
	return renderWithAuth(<CreateWorkspacePageExperimental />, {
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

describe("CreateWorkspacePageExperimental", () => {
	let mockWebSocket: WebSocket;
	let publisher: MockPublisher;

	beforeEach(() => {
		jest.clearAllMocks();

		jest.spyOn(API, "getTemplate").mockResolvedValue(MockTemplate);
		jest.spyOn(API, "getTemplateVersionExternalAuth").mockResolvedValue([]);
		jest.spyOn(API, "getTemplateVersionPresets").mockResolvedValue([]);
		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
		jest.spyOn(API, "checkAuthorization").mockResolvedValue({});

		jest
			.spyOn(API, "templateVersionDynamicParameters")
			.mockImplementation((versionId, _ownerId, callbacks) => {
				const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
				mockWebSocket = socket;
				publisher = pub;

				socket.addEventListener("message", (event) => {
					callbacks.onMessage(JSON.parse(event.data));
				});
				socket.addEventListener("error", (event) => {
					callbacks.onError((event as ErrorEvent).error);
				});
				socket.addEventListener("close", () => {
					callbacks.onClose();
				});

				setTimeout(() => {
					publisher.publishOpen(new Event("open"));
					publisher.publishMessage(
						new MessageEvent("message", {
							data: JSON.stringify(mockDynamicParametersResponse),
						}),
					);
				}, 10);

				return mockWebSocket;
			});
	});

	afterEach(() => {
		mockWebSocket?.close();
		jest.restoreAllMocks();
	});

	describe("WebSocket Integration", () => {
		it("establishes WebSocket connection and receives initial parameters", async () => {
			renderCreateWorkspacePageExperimental();

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
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(screen.getByText("Tags")).toBeInTheDocument();
			});
		});

		it("sends parameter updates via WebSocket when form values change", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			expect(mockWebSocket.send).toBeDefined();

			const instanceTypeSelect = screen.getByRole("combobox", {
				name: /instance type/i,
			});
			expect(instanceTypeSelect).toBeInTheDocument();

			await waitFor(async () => {
				await userEvent.click(instanceTypeSelect);
			});

			let mediumOption: Element | null = null;
			await waitFor(() => {
				mediumOption = screen.queryByRole("option", { name: /t3\.medium/i });
				expect(mediumOption).toBeTruthy();
			});

			await waitFor(async () => {
				await userEvent.click(mediumOption!);
			});

			expect(mockWebSocket.send).toHaveBeenCalledWith(
				expect.stringContaining('"instance_type":"t3.medium"'),
			);
		});

		it("handles WebSocket error gracefully", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("error", (event) => {
						callbacks.onError((event as ErrorEvent).error);
					});

					// Simulate error
					setTimeout(() => {
						publisher.publishError(
							new ErrorEvent("error", {
								error: new Error("Connection failed"),
							}),
						);
					}, 10);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(screen.getByText(/connection failed/i)).toBeInTheDocument();
			});
		});

		it("handles WebSocket close event", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("close", () => {
						callbacks.onClose();
					});

					setTimeout(() => {
						publisher.publishClose(new CloseEvent("close"));
					}, 10);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();

			await waitFor(() => {
				expect(
					screen.getByText(/websocket connection.*unexpectedly closed/i),
				).toBeInTheDocument();
			});
		});

		it("only parameters from latest response are displayed", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					setTimeout(() => {
						publisher.publishOpen(new Event("open"));
						publisher.publishMessage(
							new MessageEvent("message", {
								data: JSON.stringify({
									id: 0,
									parameters: [mockDropdownParameter],
									diagnostics: [],
								}),
							}),
						);
					}, 0);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			const response1: DynamicParametersResponse = {
				id: 1,
				parameters: [mockDropdownParameter],
				diagnostics: [],
			};
			const response2: DynamicParametersResponse = {
				id: 4,
				parameters: [mockSliderParameter],
				diagnostics: [],
			};

			setTimeout(() => {
				publisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response1) }),
				);

				publisher.publishMessage(
					new MessageEvent("message", { data: JSON.stringify(response2) }),
				);
			}, 0);

			await waitFor(() => {
				expect(screen.queryByText("CPU Count")).toBeInTheDocument();
				expect(screen.queryByText("Instance Type")).not.toBeInTheDocument();
			});
		});
	});

	describe("Dynamic Parameter Types", () => {
		it("renders dropdown parameter with options", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
				expect(
					screen.getByRole("combobox", { name: /instance type/i }),
				).toBeInTheDocument();
			});

			const select = screen.getByRole("combobox", { name: /instance type/i });

			await waitFor(async () => {
				await userEvent.click(select);
			});

			expect(
				screen.getByRole("option", { name: /t3\.micro/i }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: /t3\.small/i }),
			).toBeInTheDocument();
			expect(
				screen.getByRole("option", { name: /t3\.medium/i }),
			).toBeInTheDocument();
		});

		it("renders number parameter with slider", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("CPU Count")).toBeInTheDocument();
			});

			await waitFor(() => {
				const numberInput = screen.getByDisplayValue("2");
				expect(numberInput).toBeInTheDocument();
			});
		});

		it("renders boolean parameter with switch", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Enable Monitoring")).toBeInTheDocument();
				expect(
					screen.getByRole("switch", { name: /enable monitoring/i }),
				).toBeInTheDocument();
			});
		});

		it("renders list parameter with tag input", async () => {
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Tags")).toBeInTheDocument();
				expect(
					screen.getByRole("textbox", { name: /tags/i }),
				).toBeInTheDocument();
			});
		});

		it("displays parameter validation errors", async () => {
			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					setTimeout(() => {
						publisher.publishMessage(
							new MessageEvent("message", {
								data: JSON.stringify(mockDynamicParametersResponseWithError),
							}),
						);
					}, 10);

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();
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
				parameters: [validationParameter],
				diagnostics: [],
			};

			const mockResponseWithError: DynamicParametersResponse = {
				id: 2,
				parameters: [
					{
						...validationParameter,
						value: { value: "200", valid: false },
						diagnostics: [
							{
								severity: "error",
								summary: "Invalid parameter value according to 'validation' block",
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

			jest
				.spyOn(API, "templateVersionDynamicParameters")
				.mockImplementation((versionId, _ownerId, callbacks) => {
					const [socket, pub] = createMockWebSocket(`ws://test/${versionId}`);
					mockWebSocket = socket;
					publisher = pub;

					socket.addEventListener("message", (event) => {
						callbacks.onMessage(JSON.parse(event.data));
					});

					setTimeout(() => {
						publisher.publishOpen(new Event("open"));

						publisher.publishMessage(
							new MessageEvent("message", {
								data: JSON.stringify(mockResponseInitial),
							}),
						);
					}, 10);

					const originalSend = socket.send;
					socket.send = jest.fn((data) => {
						originalSend.call(socket, data);

						if (typeof data === "string" && data.includes('"200"')) {
							setTimeout(() => {
								publisher.publishMessage(
									new MessageEvent("message", {
										data: JSON.stringify(mockResponseWithError),
									}),
								);
							}, 10);
						}
					});

					return mockWebSocket;
				});

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Invalid Parameter")).toBeInTheDocument();
			});

			const numberInput = screen.getByDisplayValue("50");
			expect(numberInput).toBeInTheDocument();

			await waitFor(async () => {
				await userEvent.clear(numberInput);
				await userEvent.type(numberInput, "200");
			});

			await waitFor(() => {
				expect(screen.getByDisplayValue("200")).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(screen.getByText("Invalid parameter value according to 'validation' block")).toBeInTheDocument();
			});

			await waitFor(() => {
				expect(
					screen.getByText("value 200 is more than the maximum 100"),
				).toBeInTheDocument();
			});

			const errorElement = screen.getByText("value 200 is more than the maximum 100");
			expect(errorElement.closest('div')).toHaveClass("text-content-destructive");
		});
	});

	describe("External Authentication", () => {
		it("displays external auth providers", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(
					screen.getByRole("button", { name: /login with github/i }),
				).toBeInTheDocument();
			});
		});

		it("shows authenticated state for connected providers", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([
					MockTemplateVersionExternalAuthGithubAuthenticated,
				]);

			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("GitHub")).toBeInTheDocument();
				expect(screen.getByText(/authenticated/i)).toBeInTheDocument();
			});
		});

		it("prevents auto-creation when required external auth is missing", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([MockTemplateVersionExternalAuthGithub]);

			renderCreateWorkspacePageExperimental(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(
					screen.getByText(
						/external authentication providers that are not connected/i,
					),
				).toBeInTheDocument();
				expect(
					screen.getByText(/auto-creation has been disabled/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("Auto-creation Mode", () => {
		// it("auto create a workspace if uses mode=auto", async () => {
		// 	const param = "first_parameter";
		// 	const paramValue = "It works!";
		// 	const createWorkspaceSpy = jest.spyOn(API, "createWorkspace");

		// 	renderWithAuth(<CreateWorkspacePageExperimental />, {
		// 		route: `/templates/default/${MockTemplate.name}/workspace?param.${param}=${paramValue}&mode=auto`,
		// 		path: "/templates/:organization/:template/workspace",
		// 	});

		// 	await waitForLoaderToBeRemoved();

		// 	// Wait for WebSocket parameters to load first
		// 	await waitFor(() => {
		// 		expect(screen.getByText("Instance Type")).toBeInTheDocument();
		// 	});

		// 	// Debug what's happening
		// 	console.log("createWorkspace spy call count:", createWorkspaceSpy.mock.calls.length);
		// 	console.log("createWorkspace spy calls:", createWorkspaceSpy.mock.calls);

		// 	// Wait for auto-creation with extended timeout
		// 	await waitFor(
		// 		() => {
		// 			expect(createWorkspaceSpy).toHaveBeenCalledWith(
		// 				"me",
		// 				expect.objectContaining({
		// 					template_version_id: MockTemplate.active_version_id,
		// 					rich_parameter_values: [
		// 						expect.objectContaining({
		// 							name: param,
		// 							source: "url",
		// 							value: paramValue,
		// 						}),
		// 					],
		// 				}),
		// 			);
		// 		},
		// 		{ timeout: 10000 }
		// 	);
		// });

		it("falls back to form mode when auto-creation fails", async () => {
			jest
				.spyOn(API, "getTemplateVersionExternalAuth")
				.mockResolvedValue([
					MockTemplateVersionExternalAuthGithubAuthenticated,
				]);
			jest
				.spyOn(API, "createWorkspace")
				.mockRejectedValue(new Error("Auto-creation failed"));

			renderCreateWorkspacePageExperimental(
				`/templates/${MockTemplate.name}/workspace?mode=auto`,
			);

			await waitForLoaderToBeRemoved();

			// Wait for WebSocket parameters to load
			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			// Wait for fallback to form mode after auto-creation fails
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
			renderCreateWorkspacePageExperimental();
			await waitForLoaderToBeRemoved();

			await waitFor(() => {
				expect(screen.getByText("Instance Type")).toBeInTheDocument();
			});

			const nameInput = screen.getByRole("textbox", {
				name: /workspace name/i,
			});
			await waitFor(async () => {
				await userEvent.clear(nameInput);
				await userEvent.type(nameInput, "my-test-workspace");
			});

			const createButton = screen.getByRole("button", {
				name: /create workspace/i,
			});
			await waitFor(async () => {
				await userEvent.click(createButton);
			});

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
						],
					}),
				);
			});
		});

		// 	it("displays creation progress", async () => {
		// 		jest
		// 			.spyOn(API, "createWorkspace")
		// 			.mockImplementation(
		// 				() =>
		// 					new Promise((resolve) =>
		// 						setTimeout(() => resolve(MockWorkspace), 1000),
		// 					),
		// 			);

		// 		renderCreateWorkspacePageExperimental();
		// 		await waitForLoaderToBeRemoved();

		// 		const nameInput = screen.getByRole("textbox", {
		// 			name: /workspace name/i,
		// 		});
		// 		await userEvent.clear(nameInput);
		// 		await userEvent.type(nameInput, "my-test-workspace");

		// 		// Submit form
		// 		const createButton = screen.getByRole("button", {
		// 			name: /create workspace/i,
		// 		});
		// 		await userEvent.click(createButton);

		// 		// Should show loading state
		// 		expect(screen.getByText(/creating/i)).toBeInTheDocument();
		// 		expect(createButton).toBeDisabled();
		// 	});

		// 	it("handles creation errors", async () => {
		// 		const errorMessage = "Failed to create workspace";
		// 		jest
		// 			.spyOn(API, "createWorkspace")
		// 			.mockRejectedValue(new Error(errorMessage));

		// 		renderCreateWorkspacePageExperimental();
		// 		await waitForLoaderToBeRemoved();

		// 		const nameInput = screen.getByRole("textbox", {
		// 			name: /workspace name/i,
		// 		});
		// 		await userEvent.clear(nameInput);
		// 		await userEvent.type(nameInput, "my-test-workspace");

		// 		// Submit form
		// 		const createButton = screen.getByRole("button", {
		// 			name: /create workspace/i,
		// 		});
		// 		await userEvent.click(createButton);

		// 		await waitFor(() => {
		// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
		// 		});
		// 	});
		// });

		// describe("URL Parameters", () => {
		// 	it("pre-fills parameters from URL", async () => {
		// 		renderCreateWorkspacePageExperimental(
		// 			`/templates/${MockTemplate.name}/workspace?param.instance_type=t3.large&param.cpu_count=4`,
		// 		);
		// 		await waitForLoaderToBeRemoved();

		// 		await waitFor(() => {
		// 			// Verify parameters are pre-filled
		// 			// This would require checking the actual form values
		// 			expect(screen.getByText("Instance Type")).toBeInTheDocument();
		// 			expect(screen.getByText("CPU Count")).toBeInTheDocument();
		// 		});
		// 	});

		// 	it("uses custom template version when specified", async () => {
		// 		const customVersionId = "custom-version-123";

		// 		renderCreateWorkspacePageExperimental(
		// 			`/templates/${MockTemplate.name}/workspace?version=${customVersionId}`,
		// 		);

		// 		await waitFor(() => {
		// 			expect(API.templateVersionDynamicParameters).toHaveBeenCalledWith(
		// 				customVersionId,
		// 				MockUserOwner.id,
		// 				expect.any(Object),
		// 			);
		// 		});
		// 	});

		// 	it("pre-fills workspace name from URL", async () => {
		// 		const workspaceName = "my-custom-workspace";

		// 		renderCreateWorkspacePageExperimental(
		// 			`/templates/${MockTemplate.name}/workspace?name=${workspaceName}`,
		// 		);
		// 		await waitForLoaderToBeRemoved();

		// 		await waitFor(() => {
		// 			const nameInput = screen.getByRole("textbox", {
		// 				name: /workspace name/i,
		// 			});
		// 			expect(nameInput).toHaveValue(workspaceName);
		// 		});
		// 	});
		// });

		// describe("Template Presets", () => {
		// 	const mockPreset = {
		// 		ID: "preset-1",
		// 		Name: "Development",
		// 		description: "Development environment preset",
		// 		Parameters: [
		// 			{ Name: "instance_type", Value: "t3.small" },
		// 			{ Name: "cpu_count", Value: "2" },
		// 		],
		// 		Default: false,
		// 	};

		// 	it("displays available presets", async () => {
		// 		jest
		// 			.spyOn(API, "getTemplateVersionPresets")
		// 			.mockResolvedValue([mockPreset]);

		// 		renderCreateWorkspacePageExperimental();
		// 		await waitForLoaderToBeRemoved();

		// 		await waitFor(() => {
		// 			expect(screen.getByText("Development")).toBeInTheDocument();
		// 			expect(
		// 				screen.getByText("Development environment preset"),
		// 			).toBeInTheDocument();
		// 		});
		// 	});

		// 	it("applies preset parameters when selected", async () => {
		// 		jest
		// 			.spyOn(API, "getTemplateVersionPresets")
		// 			.mockResolvedValue([mockPreset]);

		// 		renderCreateWorkspacePageExperimental();
		// 		await waitForLoaderToBeRemoved();

		// 		// Select preset
		// 		const presetButton = screen.getByRole("button", { name: /development/i });
		// 		await userEvent.click(presetButton);

		// 		// Verify parameters are sent via WebSocket
		// 		await waitFor(() => {
		// 			expect(mockWebSocket.send).toHaveBeenCalledWith(
		// 				expect.stringContaining('"instance_type":"t3.small"'),
		// 			);
		// 			expect(mockWebSocket.send).toHaveBeenCalledWith(
		// 				expect.stringContaining('"cpu_count":"2"'),
		// 			);
		// 		});
		// 	});
		// });
	});

	// describe("Navigation", () => {
	// 	it("navigates back when cancel is clicked", async () => {
	// 		const { router } = renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const cancelButton = screen.getByRole("button", { name: /cancel/i });
	// 		await userEvent.click(cancelButton);

	// 		expect(router.state.location.pathname).not.toBe(
	// 			`/templates/${MockTemplate.name}/workspace`,
	// 		);
	// 	});

	// 	it("navigates to workspace after successful creation", async () => {
	// 		const { router } = renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		const nameInput = screen.getByRole("textbox", {
	// 			name: /workspace name/i,
	// 		});
	// 		await userEvent.clear(nameInput);
	// 		await userEvent.type(nameInput, "my-test-workspace");

	// 		// Submit form
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(router.state.location.pathname).toBe(
	// 				`/@${MockWorkspace.owner_name}/${MockWorkspace.name}`,
	// 			);
	// 		});
	// 	});
	// });

	// describe("Error Handling", () => {
	// 	it("displays template loading errors", async () => {
	// 		const errorMessage = "Template not found";
	// 		jest.spyOn(API, "getTemplate").mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("displays permission errors", async () => {
	// 		const errorMessage = "Insufficient permissions";
	// 		jest
	// 			.spyOn(API, "checkAuthorization")
	// 			.mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});
	// 	});

	// 	it("allows error reset", async () => {
	// 		const errorMessage = "Creation failed";
	// 		jest
	// 			.spyOn(API, "createWorkspace")
	// 			.mockRejectedValue(new Error(errorMessage));

	// 		renderCreateWorkspacePageExperimental();
	// 		await waitForLoaderToBeRemoved();

	// 		// Trigger error
	// 		const createButton = screen.getByRole("button", {
	// 			name: /create workspace/i,
	// 		});
	// 		await userEvent.click(createButton);

	// 		await waitFor(() => {
	// 			expect(screen.getByText(errorMessage)).toBeInTheDocument();
	// 		});

	// 		// Reset error
	// 		jest.spyOn(API, "createWorkspace").mockResolvedValue(MockWorkspace);
	// 		const errorBanner = screen.getByRole("alert");
	// 		const tryAgainButton = within(errorBanner).getByRole("button", {
	// 			name: /try again/i,
	// 		});
	// 		await userEvent.click(tryAgainButton);

	// 		await waitFor(() => {
	// 			expect(screen.queryByText(errorMessage)).not.toBeInTheDocument();
	// 		});
	// 	});
	// });
});

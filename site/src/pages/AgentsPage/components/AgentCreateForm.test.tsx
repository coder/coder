import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { type FC, type PropsWithChildren, StrictMode } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import { permittedOrganizations } from "#/api/queries/organizations";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockOrganization2,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import themes, { DEFAULT_THEME } from "#/theme";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	AgentCreateForm,
	emptyInputStorageKey,
	selectedOrganizationIdStorageKey,
	selectedWorkspaceIdStorageKey,
} from "./AgentCreateForm";

type WrapperProps = PropsWithChildren<{
	queryClient?: ReturnType<typeof createTestQueryClient>;
}>;

const Wrapper: FC<WrapperProps> = ({
	children,
	queryClient = createTestQueryClient(),
}) => {
	return (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: [],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization, MockOrganization2],
					showOrganizations: true,
					canViewOrganizationSettings: false,
				}}
			>
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					<TooltipProvider>
						<MemoryRouter>{children}</MemoryRouter>
					</TooltipProvider>
				</ThemeOverride>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

const formProps = {
	onCreateChat: vi.fn(),
	isCreating: false,
	createError: undefined,
	canCreateChat: true,
	canConfigureAgentSetup: false,
	aiGatewayDisabled: false,
	workspaceCount: 0,
	workspaceOptions: [],
	workspacesError: undefined,
	isWorkspacesLoading: false,
};

const chatCreatePermission = {
	object: { resource_type: "chat", owner_id: "me" },
	action: "create",
} as const;

const userDraftAttachments = JSON.stringify([
	{
		fileId: "user-draft-file",
		fileName: "notes.txt",
		fileType: "text/plain",
		lastModified: 1000,
		organizationId: MockDefaultOrganization.id,
	},
]);

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreateForm", () => {
	it("keeps the remembered organization and workspace while a project locks another", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockDefaultOrganization.id,
		);
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-default-org");
		const mcpRequests: string[] = [];
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
				);
			}),
			http.get(
				"/api/v2/organizations/:organization/mcp-servers",
				({ params }) => {
					mcpRequests.push(String(params.organization));
					return HttpResponse.json([]);
				},
			),
		);

		const { rerender } = render(
			<Wrapper>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);
		await waitFor(() => {
			expect(mcpRequests).toContain(MockDefaultOrganization.id);
		});
		// The same form instance survives navigating between the plain composer
		// and a cached project page, as it does under the router.
		rerender(
			<Wrapper>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);

		mcpRequests.length = 0;
		rerender(
			<Wrapper>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(mcpRequests).toContain(MockDefaultOrganization.id);
		});
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);
	});

	it("clears a revoked workspace after leaving a lock on the same organization", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockOrganization2.id,
		);
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-org2");
		let permittedOrganizationIds = new Set([
			MockDefaultOrganization.id,
			MockOrganization2.id,
		]);
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(
						Object.keys(checks).map((key) => [
							key,
							permittedOrganizationIds.has(key),
						]),
					),
				);
			}),
		);
		const queryClient = createTestQueryClient();
		const { rerender } = render(
			<Wrapper queryClient={queryClient}>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);
		await waitFor(() => {
			expect(
				queryClient.getQueryData(
					permittedOrganizations(chatCreatePermission).queryKey,
				),
			).toBeDefined();
		});

		rerender(
			<Wrapper queryClient={queryClient}>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);
		permittedOrganizationIds = new Set([MockDefaultOrganization.id]);
		await queryClient.invalidateQueries({
			queryKey: permittedOrganizations(chatCreatePermission).queryKey,
		});

		await waitFor(() => {
			expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBeNull();
		});
	});

	it("denies access when the locked organization is not permitted", async () => {
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-default-org");
		const persistedAttachments = JSON.stringify([
			{
				fileId: "persisted-file",
				fileName: "notes.txt",
				fileType: "text/plain",
				lastModified: 1000,
				organizationId: MockDefaultOrganization.id,
			},
		]);
		localStorage.setItem(persistedAttachmentsStorageKey, persistedAttachments);
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(
						Object.keys(checks).map((key) => [
							key,
							key === MockDefaultOrganization.id,
						]),
					),
				);
			}),
		);

		render(
			<Wrapper>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);

		await screen.findByText(
			"You don't have permission to create chats in this project's organization.",
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			persistedAttachments,
		);
	});
});

describe("AgentCreateForm prefill", () => {
	it("uploads the attachment once and sends it with the message, leaving the draft alone", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [MockDefaultChatModel],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		});
		vi.spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockResolvedValue(MockUnsetUserChatPersonalModelOverrides);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		localStorage.setItem(persistedAttachmentsStorageKey, userDraftAttachments);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const user = userEvent.setup();

		render(
			<StrictMode>
				<AppProviders queryClient={createTestQueryClient()}>
					<DashboardContext.Provider
						value={{
							entitlements: MockEntitlements,
							experiments: [],
							appearance: MockAppearanceConfig,
							buildInfo: MockBuildInfo,
							organizations: [MockDefaultOrganization],
							showOrganizations: false,
							canViewOrganizationSettings: false,
						}}
					>
						<AgentCreateForm
							onCreateChat={onCreateChat}
							isCreating={false}
							createError={undefined}
							canCreateChat
							canConfigureAgentSetup={false}
							workspaceCount={0}
							workspaceOptions={[]}
							workspacesError={undefined}
							isWorkspacesLoading={false}
							prefill={{
								message: "Why did this build fail?",
								attachment: {
									name: "workspace-build-logs.txt",
									text: "Error: exit status 1\n",
								},
							}}
						/>
					</DashboardContext.Provider>
				</AppProviders>
			</StrictMode>,
		);

		const sendButton = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			"Error: exit status 1\n",
		);
		expect(uploadOrganizationId).toBe(MockDefaultOrganization.id);
		expect(onCreateChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				message: "Why did this build fail?",
				fileIDs: ["uploaded-logs"],
				organizationId: MockDefaultOrganization.id,
			}),
		);
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft the user typed earlier",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			userDraftAttachments,
		);
	});
});

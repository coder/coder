import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	mcpServerConfigsKey,
	organizationChatModelsKey,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	DashboardContext,
	type DashboardValue,
} from "#/modules/dashboard/DashboardProvider";
import type { Permissions } from "#/modules/permissions";
import { MockChat, MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockAIProviderOpenAI,
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockNoPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { mcpSelectionStorageKey } from "../components/MCPServerPicker";
import {
	type AgentChatSettings,
	AgentChatSettingsProvider,
	useAgentChatSettings,
} from "./AgentChatSettingsContext";

// The provider only reads permissions from the auth context.
let permissions: Permissions = MockNoPermissions;

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions,
		signOut: vi.fn(),
	}),
}));

const ORGANIZATION_ID = "settings-organization";

const HISTORICAL_MODEL_ID = "model-historical";
const DEFAULT_MODEL_ID = "model-default";
const ALTERNATE_MODEL_ID = "model-alternate";

const buildModel = (
	id: string,
	overrides: Partial<TypesGen.ChatModel> = {},
): TypesGen.ChatModel => ({
	...MockChatModel,
	id,
	organization_id: ORGANIZATION_ID,
	ai_provider_id: MockChatModelProviderDescriptor.id,
	model: id,
	display_name: id,
	is_default: false,
	...overrides,
});

const buildCatalog = (
	models: readonly TypesGen.ChatModel[],
): TypesGen.OrganizationChatModelsResponse => ({
	models: [...models],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
});

const DEFAULT_ON_SERVER: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-default-on",
	organization_id: ORGANIZATION_ID,
	slug: "mcp-default-on",
	availability: "default_on",
};

const OPT_IN_SERVER: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-opt-in",
	organization_id: ORGANIZATION_ID,
	slug: "mcp-opt-in",
	availability: "default_off",
};

const MCP_SERVERS = [DEFAULT_ON_SERVER, OPT_IN_SERVER];

const DEFAULT_CATALOG = buildCatalog([buildModel(HISTORICAL_MODEL_ID)]);

// A provider the catalog reports as available, so it counts as configured.
const CONFIGURED_PROVIDER: TypesGen.AIProvider = {
	...MockAIProviderOpenAI,
	id: MockChatModelProviderDescriptor.id,
	enabled: true,
};

const LATE_SERVER: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-authorized",
	organization_id: ORGANIZATION_ID,
	slug: "mcp-authorized",
	availability: "default_off",
};

const buildChat = (overrides: Partial<TypesGen.Chat> = {}): TypesGen.Chat => ({
	...MockChat,
	organization_id: ORGANIZATION_ID,
	last_model_config_id: HISTORICAL_MODEL_ID,
	...overrides,
});

const dashboardValue: DashboardValue = {
	entitlements: MockEntitlements,
	experiments: [],
	appearance: MockAppearanceConfig,
	buildInfo: MockBuildInfo,
	// The chat organization is not the default one, so the legacy unscoped MCP
	// selection stays out of these cases.
	organizations: [
		MockDefaultOrganization,
		{
			...MockDefaultOrganization,
			id: ORGANIZATION_ID,
			name: "settings-org",
			display_name: "Settings Org",
			is_default: false,
		},
	],
	showOrganizations: true,
	canViewOrganizationSettings: false,
};

/** The settings a consumer observed when the user asked it to report. */
type SettingsReport = {
	selectedModel: string;
	selectedMCPServerIds: readonly string[];
	mcpServerIds: readonly string[];
	hasPickedReasoningEffort: boolean;
	canConfigureAgentSetup: boolean;
	providerCount: number | undefined;
};

const toReport = (settings: AgentChatSettings): SettingsReport => ({
	selectedModel: settings.selectedModel,
	selectedMCPServerIds: settings.selectedMCPServerIds,
	mcpServerIds: settings.mcpServers.map((server) => server.id),
	hasPickedReasoningEffort: settings.hasPickedReasoningEffort,
	canConfigureAgentSetup: settings.canConfigureAgentSetup,
	providerCount: settings.providerCount,
});

/**
 * A consumer that reports what it currently reads from the context, so a
 * change another consumer made is observable without inspecting the provider.
 */
const SettingsReporter: FC<{ onReport: (report: SettingsReport) => void }> = ({
	onReport,
}) => {
	const settings = useAgentChatSettings();
	return (
		<button type="button" onClick={() => onReport(toReport(settings))}>
			Report settings
		</button>
	);
};

/** A separate consumer that changes the settings the reporter reads. */
const SettingsEditor: FC = () => {
	const settings = useAgentChatSettings();
	return (
		<>
			<button
				type="button"
				onClick={() => settings.onModelChange(ALTERNATE_MODEL_ID)}
			>
				Use alternate model
			</button>
			<button
				type="button"
				onClick={() => settings.onReasoningEffortChange("high")}
			>
				Use high reasoning effort
			</button>
			<button type="button" onClick={settings.resetPickedReasoningEffort}>
				Reset reasoning effort pick
			</button>
			<button type="button" onClick={() => settings.onMCPSelectionChange([])}>
				Clear MCP servers
			</button>
			<button
				type="button"
				onClick={() => settings.onMCPSelectionChange([OPT_IN_SERVER.id])}
			>
				Use opt-in MCP server
			</button>
			<button
				type="button"
				onClick={() => settings.onMCPAuthComplete(OPT_IN_SERVER.id)}
			>
				Finish MCP auth
			</button>
		</>
	);
};

const renderSettings = ({
	chat,
	catalog = DEFAULT_CATALOG,
}: {
	chat: TypesGen.Chat | undefined;
	catalog?: TypesGen.OrganizationChatModelsResponse;
}) => {
	const queryClient = createTestQueryClient();
	// Seed the real query cache so the provider resolves its settings on the
	// first render, the way it does from a warm cache in the app.
	queryClient.setQueryData(organizationChatModelsKey(ORGANIZATION_ID), catalog);
	queryClient.setQueryData(mcpServerConfigsKey(ORGANIZATION_ID), MCP_SERVERS);

	const onReport = vi.fn<(report: SettingsReport) => void>();
	const Wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider value={dashboardValue}>
				{children}
			</DashboardContext.Provider>
		</QueryClientProvider>
	);

	render(
		<Wrapper>
			<AgentChatSettingsProvider chat={chat}>
				<SettingsEditor />
				<SettingsReporter onReport={onReport} />
			</AgentChatSettingsProvider>
		</Wrapper>,
	);

	const clickButton = async (name: string) => {
		await userEvent.click(screen.getByRole("button", { name }));
	};

	const readReport = async (): Promise<SettingsReport> => {
		onReport.mockClear();
		await clickButton("Report settings");
		expect(onReport).toHaveBeenCalledTimes(1);
		return onReport.mock.calls[0][0];
	};

	return { clickButton, readReport };
};

describe("AgentChatSettingsProvider", () => {
	beforeEach(() => {
		localStorage.clear();
		permissions = MockNoPermissions;
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(
			DEFAULT_CATALOG,
		);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			...MCP_SERVERS,
		]);
		vi.spyOn(API, "getAIProviders").mockResolvedValue([CONFIGURED_PROVIDER]);
	});

	afterEach(() => {
		vi.restoreAllMocks();
		localStorage.clear();
	});

	it("prefers the chat's historical model over the organization default", async () => {
		const { readReport } = renderSettings({
			chat: buildChat(),
			catalog: buildCatalog([
				buildModel(DEFAULT_MODEL_ID, { is_default: true }),
				buildModel(HISTORICAL_MODEL_ID),
			]),
		});

		await expect(readReport()).resolves.toMatchObject({
			selectedModel: HISTORICAL_MODEL_ID,
		});
	});

	it("falls back to the organization default when the historical model is gone", async () => {
		const { readReport } = renderSettings({
			chat: buildChat({ last_model_config_id: "model-deleted" }),
			catalog: buildCatalog([
				buildModel(ALTERNATE_MODEL_ID),
				buildModel(DEFAULT_MODEL_ID, { is_default: true }),
			]),
		});

		await expect(readReport()).resolves.toMatchObject({
			selectedModel: DEFAULT_MODEL_ID,
		});
	});

	it("applies a user's model choice to other consumers immediately", async () => {
		const { clickButton, readReport } = renderSettings({
			chat: buildChat(),
			catalog: buildCatalog([
				buildModel(HISTORICAL_MODEL_ID),
				buildModel(ALTERNATE_MODEL_ID),
			]),
		});

		await expect(readReport()).resolves.toMatchObject({
			selectedModel: HISTORICAL_MODEL_ID,
		});

		await clickButton("Use alternate model");

		await expect(readReport()).resolves.toMatchObject({
			selectedModel: ALTERNATE_MODEL_ID,
		});
	});

	it("tracks a deliberate reasoning effort pick until it is reset", async () => {
		const { clickButton, readReport } = renderSettings({
			chat: buildChat(),
		});

		await expect(readReport()).resolves.toMatchObject({
			hasPickedReasoningEffort: false,
		});

		await clickButton("Use high reasoning effort");
		await expect(readReport()).resolves.toMatchObject({
			hasPickedReasoningEffort: true,
		});

		await clickButton("Reset reasoning effort pick");
		await expect(readReport()).resolves.toMatchObject({
			hasPickedReasoningEffort: false,
		});
	});

	it("keeps a chat's empty MCP selection over the saved organization one", async () => {
		localStorage.setItem(
			mcpSelectionStorageKey(ORGANIZATION_ID),
			JSON.stringify([OPT_IN_SERVER.id]),
		);

		const { readReport } = renderSettings({
			chat: buildChat({ mcp_server_ids: [] }),
		});

		await expect(readReport()).resolves.toMatchObject({
			selectedMCPServerIds: [],
		});
	});

	it("prefers a user's MCP selection over the chat's", async () => {
		const { clickButton, readReport } = renderSettings({
			chat: buildChat({ mcp_server_ids: [DEFAULT_ON_SERVER.id] }),
		});

		await clickButton("Use opt-in MCP server");

		await expect(readReport()).resolves.toMatchObject({
			selectedMCPServerIds: [OPT_IN_SERVER.id],
		});
	});

	it("persists an empty MCP selection so the opt-out survives a reload", async () => {
		const { clickButton, readReport } = renderSettings({
			chat: buildChat({ mcp_server_ids: [DEFAULT_ON_SERVER.id] }),
		});

		await clickButton("Clear MCP servers");

		await expect(readReport()).resolves.toMatchObject({
			selectedMCPServerIds: [],
		});
		expect(localStorage.getItem(mcpSelectionStorageKey(ORGANIZATION_ID))).toBe(
			"[]",
		);
	});

	it("refetches the MCP servers after an auth flow completes", async () => {
		const { clickButton, readReport } = renderSettings({ chat: buildChat() });
		const getMCPServerConfigs = vi.mocked(API.experimental.getMCPServerConfigs);
		// The server the user just authorized only appears once the provider
		// refetches, so an unchanged report would mean the refetch never ran.
		getMCPServerConfigs.mockResolvedValue([...MCP_SERVERS, LATE_SERVER]);

		await expect(readReport()).resolves.toMatchObject({
			mcpServerIds: MCP_SERVERS.map((server) => server.id),
		});

		await clickButton("Finish MCP auth");

		await vi.waitFor(async () => {
			await expect(readReport()).resolves.toMatchObject({
				mcpServerIds: [...MCP_SERVERS, LATE_SERVER].map((server) => server.id),
			});
		});
	});

	it("leaves the provider catalog query disabled without deployment permission", async () => {
		const { readReport } = renderSettings({ chat: buildChat() });

		await expect(readReport()).resolves.toMatchObject({
			canConfigureAgentSetup: false,
			providerCount: undefined,
		});
		expect(API.getAIProviders).not.toHaveBeenCalled();
	});

	it("counts configured providers for a deployment administrator", async () => {
		permissions = { ...MockNoPermissions, editDeploymentConfig: true };
		const { readReport } = renderSettings({ chat: buildChat() });

		await vi.waitFor(async () => {
			await expect(readReport()).resolves.toMatchObject({
				canConfigureAgentSetup: true,
				providerCount: 1,
			});
		});
	});

	it("rejects consumers rendered outside the provider", () => {
		const consoleError = vi
			.spyOn(console, "error")
			.mockImplementation(() => {});

		expect(() => render(<SettingsReporter onReport={vi.fn()} />)).toThrow(
			"useAgentChatSettings must be used within an AgentChatSettingsProvider",
		);

		consoleError.mockRestore();
	});
});

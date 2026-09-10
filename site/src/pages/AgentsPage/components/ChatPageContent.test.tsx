import { MessageScroller } from "@shadcn/react/message-scroller";
import { render, screen } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import {
	getPreferredProxy,
	ProxyContext,
	type ProxyContextValue,
} from "#/contexts/ProxyContext";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChat, MockChatMessage } from "#/testHelpers/chatEntities";
import {
	MockProxyLatencies,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatPageTimeline, workspaceSkillsFromChat } from "./ChatPageContent";

const skillResource = (
	name: string,
	overrides: Partial<TypesGen.ChatContextResource> = {},
): TypesGen.ChatContextResource => ({
	source: `/workspace/.agents/skills/${name}`,
	kind: "skill",
	size_bytes: 128,
	skill_name: name,
	skill_description: `${name} description`,
	status: "ok",
	...overrides,
});

const instructionResource = (): TypesGen.ChatContextResource => ({
	source: "/workspace/AGENTS.md",
	kind: "instruction_file",
	size_bytes: 64,
	status: "ok",
});

const chatWithContext = (
	context: TypesGen.ChatContext | undefined,
): TypesGen.Chat => ({ ...MockChat, context });

describe("workspaceSkillsFromChat", () => {
	it("returns undefined while the chat detail is unresolved", () => {
		expect(workspaceSkillsFromChat(undefined)).toBeUndefined();
	});

	it("returns an empty authoritative list for a resolved unpinned chat", () => {
		expect(workspaceSkillsFromChat(chatWithContext(undefined))).toEqual([]);
		expect(workspaceSkillsFromChat(chatWithContext({ dirty: false }))).toEqual(
			[],
		);
	});

	it("maps healthy skill resources to workspace skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				instructionResource(),
				skillResource("reviewer"),
				skillResource("docs"),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
			{ name: "docs", description: "docs description" },
		]);
	});

	it("keeps the first resource for duplicate skill names, matching read_skill", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [
				skillResource("reviewer", {
					source: "/workspace/.agents/skills/reviewer",
				}),
				skillResource("reviewer", {
					source: "/workspace/other/skills/reviewer",
					skill_description: "shadowed duplicate",
				}),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("omits non-ok skill resources", () => {
		const chat = chatWithContext({
			dirty: true,
			resources: [
				skillResource("reviewer"),
				skillResource("broken", { status: "unreadable", skill_name: "" }),
			],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([
			{ name: "reviewer", description: "reviewer description" },
		]);
	});

	it("returns an empty authoritative list when pinned context has no skills", () => {
		const chat = chatWithContext({
			dirty: false,
			resources: [instructionResource()],
		});
		expect(workspaceSkillsFromChat(chat)).toEqual([]);
	});
});

const WILDCARD_HOSTNAME = "*.coder.com";
const LOCALHOST_LINK = "http://localhost:3000/policy";

const proxyContextValue: ProxyContextValue = {
	latenciesLoaded: true,
	proxyLatencies: MockProxyLatencies,
	proxy: {
		...getPreferredProxy([], undefined),
		preferredWildcardHostname: WILDCARD_HOSTNAME,
	},
	proxies: [],
	isLoading: false,
	isFetched: true,
	setProxy: () => {},
	clearProxy: () => {},
	refetchProxyLatencies: () => new Date(),
};

const assistantMessage: TypesGen.ChatMessage = {
	...MockChatMessage,
	role: "assistant",
	content: [
		{ type: "text", text: `Preview it at [the app](${LOCALHOST_LINK})` },
	],
};

const renderTimeline = ({
	workspace,
	workspaceAgent,
}: {
	workspace?: TypesGen.Workspace;
	workspaceAgent?: TypesGen.WorkspaceAgent;
}) => {
	const store = createChatStore();
	store.replaceMessages([assistantMessage]);

	const Wrapper: FC<PropsWithChildren> = ({ children }) => (
		<QueryClientProvider client={createTestQueryClient()}>
			<ProxyContext.Provider value={proxyContextValue}>
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					<TooltipProvider>
						<MemoryRouter>
							<MessageScroller.Provider>{children}</MessageScroller.Provider>
						</MemoryRouter>
					</TooltipProvider>
				</ThemeOverride>
			</ProxyContext.Provider>
		</QueryClientProvider>
	);

	return render(
		<ChatPageTimeline
			organizationId={MockWorkspace.organization_id}
			store={store}
			persistedError={undefined}
			hasMoreMessages={false}
			isFetchingMoreMessages={false}
			isHydratingMessages={false}
			hasFetchMoreError={false}
			onFetchMoreMessages={async () => {}}
			workspace={workspace}
			workspaceAgent={workspaceAgent}
		/>,
		{ wrapper: Wrapper },
	);
};

describe("ChatPageTimeline", () => {
	it("rewrites transcript localhost links to the port-forward host", async () => {
		renderTimeline({
			workspace: MockWorkspace,
			workspaceAgent: MockWorkspaceAgent,
		});

		const subdomain = `3000--${MockWorkspaceAgent.name}--${MockWorkspace.name}--${MockWorkspace.owner_name}`;
		const link = await screen.findByRole("link", { name: "the app" });
		// URL parsing lowercases the generated port-forward hostname.
		expect(link.getAttribute("href")).toBe(
			`http://${WILDCARD_HOSTNAME.replace("*", subdomain)}/policy`.toLowerCase(),
		);
	});

	it("leaves localhost links alone without a workspace agent", async () => {
		renderTimeline({ workspace: MockWorkspace });

		const link = await screen.findByRole("link", { name: "the app" });
		expect(link.getAttribute("href")).toBe(LOCALHOST_LINK);
	});
});

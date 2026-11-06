import { renderHook } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage, WorkspaceAgent } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { AGENT_BROWSER_APP_SLUG } from "#/modules/apps/apps";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
} from "#/testHelpers/entities";
import { createChatStore } from "../../ChatConversation/chatStore";
import { useChatPreviewPanel, useNewPreviews } from "./PreviewPanelContext";

vi.mock("#/components/Toaster/toast", () => ({
	toast: { info: vi.fn(), dismiss: vi.fn() },
}));

const mockPreviewMessage: ChatMessage = {
	...MockChatMessage,
	id: 10,
	role: "tool",
	content: [
		{
			type: "tool-result",
			tool_call_id: "call-1",
			tool_name: "open_preview",
			result: {
				ok: true,
				workspace_id: "workspace-1",
				agent_name: "main",
				port: 3000,
				title: "Docs site",
			},
		},
	],
};

describe("useNewPreviews", () => {
	it("reports previews from new messages only", async () => {
		const store = createChatStore();
		store.replaceMessages([mockPreviewMessage]);

		const onNewPreview = vi.fn();
		renderHook(() =>
			useNewPreviews({
				store,
				initialMessages: [mockPreviewMessage],
				onNewPreview,
			}),
		);

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 9 });
		});

		// A send response can land before a lower-ID result from the stream.
		await act(async () => {
			store.upsertDurableMessage({ ...MockChatMessage, id: 13 });
		});
		expect(onNewPreview).not.toHaveBeenCalled();

		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		// Reconnects can deliver a message again.
		await act(async () => {
			store.upsertDurableMessage({ ...mockPreviewMessage, id: 12 });
		});

		expect(onNewPreview).toHaveBeenCalledOnce();
		expect(onNewPreview).toHaveBeenCalledWith(
			{
				workspaceId: "workspace-1",
				preview: { agent_name: "main", port: 3000 },
			},
			"Docs site",
		);
	});

	it("reports only the newest target", async () => {
		const store = createChatStore();
		const onNewPreview = vi.fn();
		renderHook(() =>
			useNewPreviews({
				store,
				initialMessages: [],
				onNewPreview,
			}),
		);

		const withPort = (id: number, port: number): ChatMessage => ({
			...mockPreviewMessage,
			id,
			content: [
				{
					type: "tool-result",
					tool_call_id: `call-${id}`,
					tool_name: "open_preview",
					result: {
						ok: true,
						workspace_id: "workspace-1",
						agent_name: "main",
						port,
					},
				},
			],
		});

		// Both land before a render, the newer one first.
		await act(async () => {
			store.upsertDurableMessage(withPort(21, 4000));
			store.upsertDurableMessage(withPort(20, 3000));
		});

		expect(onNewPreview).toHaveBeenCalledOnce();
		expect(onNewPreview).toHaveBeenCalledWith(
			{
				workspaceId: "workspace-1",
				preview: { agent_name: "main", port: 4000 },
			},
			"",
		);

		// A late older result must not replace the newer preview.
		await act(async () => {
			store.upsertDurableMessage(withPort(19, 5000));
		});
		expect(onNewPreview).toHaveBeenCalledOnce();
	});

	it("does not report apps", async () => {
		const store = createChatStore();
		const onNewPreview = vi.fn();
		renderHook(() =>
			useNewPreviews({ store, initialMessages: [], onNewPreview }),
		);

		await act(async () => {
			store.upsertDurableMessage({
				...mockPreviewMessage,
				content: [
					{
						type: "tool-result",
						tool_call_id: "call-app",
						tool_name: "open_preview",
						result: {
							ok: true,
							workspace_id: "workspace-1",
							agent_name: "main",
							app_slug: "code-server",
						},
					},
				],
			});
		});

		expect(onNewPreview).not.toHaveBeenCalled();
	});

	it("reports a preview that arrived before it could open", async () => {
		const store = createChatStore();
		const onNewPreview = vi.fn();
		const { rerender } = renderHook(
			({ canOpen }) =>
				useNewPreviews({
					store,
					initialMessages: [],
					onNewPreview: canOpen ? onNewPreview : undefined,
				}),
			{ initialProps: { canOpen: false } },
		);

		await act(async () => {
			store.upsertDurableMessage(mockPreviewMessage);
		});
		rerender({ canOpen: true });

		expect(onNewPreview).toHaveBeenCalledOnce();
	});
});

describe("useChatPreviewPanel", () => {
	it("offers new previews in a toast", async () => {
		const store = createChatStore();
		const showSingletonTab = vi.fn();
		const { rerender } = renderHook(
			({ workspaceId }) =>
				useChatPreviewPanel({
					chatId: "chat-1",
					store,
					initialMessages: [],
					workspace: { ...MockWorkspace, id: workspaceId },
					workspaceAgent: undefined,
					isPreviewTabSupported: true,
					isArchived: false,
					showSingletonTab,
					openWorkspaceApp: vi.fn(),
					setPreviewPanelKey: vi.fn(),
				}),
			{ initialProps: { workspaceId: "workspace-1" } },
		);

		await act(async () => {
			store.upsertDurableMessage({
				...mockPreviewMessage,
				id: 9,
				content: [
					{
						type: "tool-result",
						tool_call_id: "call-other",
						tool_name: "open_preview",
						result: {
							ok: true,
							workspace_id: "workspace-2",
							agent_name: "main",
						},
					},
				],
			});
		});
		// The chat cannot show a preview from another workspace.
		expect(toast.info).not.toHaveBeenCalled();

		await act(async () => {
			store.upsertDurableMessage(mockPreviewMessage);
		});
		expect(showSingletonTab).not.toHaveBeenCalled();
		expect(toast.info).toHaveBeenCalledOnce();
		const [message, options] = vi.mocked(toast.info).mock.calls[0] as [
			string,
			{
				id: string;
				description: string;
				action: { label: string; onClick: () => void };
			},
		];
		expect(message).toBe("Opened a preview");
		expect(options.description).toBe("Docs site");
		expect(options.action.label).toBe("Open preview");
		act(() => options.action.onClick());
		expect(showSingletonTab).toHaveBeenCalledExactlyOnceWith("preview");

		rerender({ workspaceId: "workspace-2" });
		expect(toast.dismiss).toHaveBeenCalledWith(options.id);
	});

	it("opens only subdomain apps of a running agent", () => {
		const mockPathApp = { ...MockWorkspaceApp, slug: "path-app" };
		const mockPathBrowserApp = {
			...MockWorkspaceApp,
			slug: AGENT_BROWSER_APP_SLUG,
		};
		const mockSubdomainApp = {
			...MockWorkspaceApp,
			slug: "subdomain-app",
			subdomain: true,
		};
		const openWorkspaceApp = vi.fn();
		const initialProps: { workspaceAgent?: WorkspaceAgent } = {
			workspaceAgent: {
				...MockWorkspaceAgent,
				name: "main",
				apps: [mockPathApp, mockPathBrowserApp, mockSubdomainApp],
			},
		};
		const { result, rerender } = renderHook(
			({ workspaceAgent }) =>
				useChatPreviewPanel({
					chatId: "chat-1",
					store: createChatStore(),
					initialMessages: [],
					workspace: { ...MockWorkspace, id: "workspace-1" },
					workspaceAgent,
					isPreviewTabSupported: true,
					isArchived: false,
					showSingletonTab: vi.fn(),
					openWorkspaceApp,
					setPreviewPanelKey: vi.fn(),
				}),
			{ initialProps },
		);
		const getOpenAction = (slug: string) =>
			result.current.getOpenAction?.({
				workspaceId: "workspace-1",
				app: { agent_name: "main", slug },
			});

		expect(getOpenAction(mockPathApp.slug)).toBeUndefined();
		expect(getOpenAction(mockPathBrowserApp.slug)).toBeUndefined();
		act(() => getOpenAction(mockSubdomainApp.slug)?.());
		expect(openWorkspaceApp).toHaveBeenCalledExactlyOnceWith(mockSubdomainApp);

		// A stopped workspace has no agents.
		rerender({ workspaceAgent: undefined });
		expect(getOpenAction(mockSubdomainApp.slug)).toBeUndefined();
	});

	it.each([
		{
			name: "opens results",
			workspaceId: "workspace-1",
			isArchived: false,
			opened: ["preview", "browser"],
		},
		{
			name: "ignores results from another workspace",
			workspaceId: "workspace-2",
			isArchived: false,
			opened: [],
		},
		{
			name: "ignores results in archived chats",
			workspaceId: "workspace-1",
			isArchived: true,
			opened: [],
		},
	])("$name", ({ workspaceId, isArchived, opened }) => {
		const showSingletonTab = vi.fn();
		const { result } = renderHook(() =>
			useChatPreviewPanel({
				chatId: "chat-1",
				store: createChatStore(),
				initialMessages: [],
				workspace: { ...MockWorkspace, id: "workspace-1" },
				workspaceAgent: {
					...MockWorkspaceAgent,
					name: "main",
					apps: [
						{
							...MockWorkspaceApp,
							slug: AGENT_BROWSER_APP_SLUG,
							subdomain: true,
						},
					],
				},
				isPreviewTabSupported: true,
				isArchived,
				showSingletonTab,
				openWorkspaceApp: vi.fn(),
				setPreviewPanelKey: vi.fn(),
			}),
		);

		act(() => {
			result.current.getOpenAction?.({
				workspaceId,
				preview: { agent_name: "main", port: 3000 },
			})?.();
			result.current.getOpenAction?.({
				workspaceId,
				app: { agent_name: "main", slug: AGENT_BROWSER_APP_SLUG },
			})?.();
		});
		expect(showSingletonTab.mock.calls.flat()).toEqual(opened);
	});
});

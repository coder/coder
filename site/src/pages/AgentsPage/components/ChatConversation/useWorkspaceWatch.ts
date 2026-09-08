import { useEffect, useEffectEvent, useRef } from "react";
import { useQueryClient } from "react-query";
import { watchWorkspace } from "#/api/api";
import { invalidateChatEntity } from "#/api/queries/chats";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import {
	isChatAgentBindingUnresolved,
	isWatchedWorkspaceViewUnchanged,
} from "./watchedWorkspace";

export function useWorkspaceWatch({
	workspaceId,
	agentId,
	chatAgentId,
}: {
	workspaceId: string | undefined;
	agentId: string | undefined;
	chatAgentId: string | undefined;
}): void {
	const queryClient = useQueryClient();
	const agentBindingRefetchKeyRef = useRef<string | undefined>(undefined);
	// Subscribe to live workspace updates so that agent status changes
	// (e.g. connected/disconnected) are reflected without a page refresh.
	const applyWatchedWorkspaceUpdate = useEffectEvent(
		(watchedWorkspaceId: string, next: TypesGen.Workspace) => {
			queryClient.setQueryData<TypesGen.Workspace | undefined>(
				workspaceByIdKey(watchedWorkspaceId),
				(prev) => {
					// Return the same reference when nothing the UI
					// reads has changed. This prevents react-query
					// from notifying subscribers and avoids a full
					// AgentChatPage re-render on every heartbeat.
					if (
						prev &&
						isWatchedWorkspaceViewUnchanged(prev, next, chatAgentId)
					) {
						return prev;
					}
					return next;
				},
			);
			// Refetch once per chat/build/binding key for immediate repair
			// after a rebuild; the chat query's refetchInterval owns retries
			// when repair fails, so the latch never blocks recovery.
			if (!agentId || !isChatAgentBindingUnresolved(next, chatAgentId)) {
				return;
			}
			const refetchKey = `${agentId}:${next.latest_build.id}:${chatAgentId ?? ""}`;
			if (agentBindingRefetchKeyRef.current === refetchKey) {
				return;
			}
			agentBindingRefetchKeyRef.current = refetchKey;
			void invalidateChatEntity(queryClient, agentId);
		},
	);
	useEffect(() => {
		if (!workspaceId) {
			return;
		}
		return createReconnectingWebSocket({
			connect() {
				const socket = watchWorkspace(workspaceId);
				socket.addEventListener("message", (event) => {
					if (event.parseError) {
						return;
					}
					if (event.parsedMessage.type === "data") {
						applyWatchedWorkspaceUpdate(
							workspaceId,
							event.parsedMessage.data as TypesGen.Workspace,
						);
					}
				});
				return socket;
			},
			onOpen() {
				// Refetch workspace data on reconnection to cover
				// events missed while disconnected. Also fires on the
				// initial connection (harmless, may deduplicate with
				// the in-flight useQuery fetch).
				void queryClient.invalidateQueries({
					queryKey: workspaceByIdKey(workspaceId),
				});
			},
		});
	}, [workspaceId, queryClient]);
}

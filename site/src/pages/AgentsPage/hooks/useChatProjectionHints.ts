import { useEffect, useEffectEvent, useRef } from "react";
import { useQueryClient } from "react-query";
import {
	addChildToParentInCache,
	cancelCachedChat,
	cancelChatListRefetches,
	getCachedChat,
	invalidateCachedChat,
	invalidateCachedChatDiff,
	invalidateChatCollectionQueries,
	mergeWatchedChatIntoCaches,
	mergeWatchedChatMetadataIntoDetail,
	prependToInfiniteChatsCache,
	readLoadedChatStatusMap,
	refetchActiveChatMetadataProjections,
	refetchDirtyChatMetadataProjections,
	removeDeletedChatFamily,
	repairParentChatProjection,
} from "#/api/queries/chats";
import { maybePlayChime } from "../utils/chime";
import {
	createChatProjectionHintFreshnessCoordinator,
	reconcileChatProjectionHint,
	subscribeChatProjectionHints,
} from "./chatProjectionHints";

export const useChatProjectionHints = (
	activeChatID: string | undefined,
	enabled = true,
): void => {
	const queryClient = useQueryClient();
	const statusByChatIDRef = useRef<ReturnType<typeof readLoadedChatStatusMap>>(
		new Map(),
	);
	const getActiveChatID = useEffectEvent(() => activeChatID);

	useEffect(() => {
		if (!enabled) {
			return;
		}

		statusByChatIDRef.current = readLoadedChatStatusMap(queryClient);

		const reconcileLiveHint = (
			hint: Parameters<typeof reconcileChatProjectionHint>[0]["hint"],
		) => {
			reconcileChatProjectionHint({
				hint,
				activeChatID: getActiveChatID(),
				ports: {
					getPreviousStatus: (chatID) => statusByChatIDRef.current.get(chatID),
					playChime: maybePlayChime,
					removeDeletedChat: (chat) => {
						removeDeletedChatFamily(queryClient, chat);
						statusByChatIDRef.current.delete(chat.id);
					},
					invalidateDiff: (chatID) => {
						void invalidateCachedChatDiff(queryClient, chatID);
					},
					cancelListRefetches: () => {
						void cancelChatListRefetches(queryClient);
					},
					hasCachedDetail: (chatID) =>
						getCachedChat(queryClient, chatID) !== undefined,
					cancelDetailRefetch: (chatID) => {
						void cancelCachedChat(queryClient, chatID);
					},
					addChild: (chat, parentID) => {
						addChildToParentInCache(queryClient, chat, parentID);
					},
					prependRoot: (chat) => {
						prependToInfiniteChatsCache(queryClient, chat);
					},
					mergeProjection: (chat, eventKind, currentActiveChatID) => {
						const options = {
							eventKind,
							activeChatId: currentActiveChatID,
						};
						mergeWatchedChatIntoCaches(queryClient, chat, options);
						mergeWatchedChatMetadataIntoDetail(queryClient, chat, options);
					},
					invalidateCollections: () => {
						void invalidateChatCollectionQueries(queryClient);
					},
					invalidateDetail: (chatID) => {
						void invalidateCachedChat(queryClient, chatID);
					},
					repairParent: (parentID) => {
						void repairParentChatProjection(queryClient, parentID).catch(
							(error) => {
								console.warn("Failed to repair parent chat projection:", error);
							},
						);
					},
				},
			});
			if (hint.kind !== "deleted") {
				statusByChatIDRef.current.set(hint.chat.id, hint.chat.status);
			}
		};

		const coordinator = createChatProjectionHintFreshnessCoordinator({
			reconcileLiveHint,
			resynchronizeBaseline: async () => {
				await refetchActiveChatMetadataProjections(queryClient);
				statusByChatIDRef.current = readLoadedChatStatusMap(queryClient);
			},
			resynchronizeDirty: async (dirty) => {
				await refetchDirtyChatMetadataProjections(
					queryClient,
					new Set(dirty.keys()),
				);
				statusByChatIDRef.current = readLoadedChatStatusMap(queryClient);
			},
			onError: (error) => {
				console.warn("Failed to resynchronize chat projections:", error);
			},
		});
		const disposeSubscription = subscribeChatProjectionHints({
			onHint: (hint, connectionEpoch) => {
				coordinator.onHint(hint, connectionEpoch);
			},
			onOpen: (connectionEpoch) => {
				coordinator.onOpen(connectionEpoch);
			},
			onDisconnect: (connectionEpoch) => {
				coordinator.onDisconnect(connectionEpoch);
			},
			onDecodeError: (error, connectionEpoch) => {
				console.warn("Failed to decode chat projection hint:", error);
				coordinator.onDecodeError(connectionEpoch);
			},
		});

		return () => {
			coordinator.dispose();
			disposeSubscription();
		};
	}, [enabled, queryClient]);
};

import { useEffect } from "react";
import { isMac } from "#/utils/platform";
import { isLetterKey } from "../utils/keyboardShortcuts";

const CHAT_ROW_SELECTOR = '[data-testid^="agents-tree-node-"]';
const COMPOSER_SELECTOR = '[data-testid="chat-message-input"]';
const DIALOG_SELECTOR = '[role="dialog"]';

/**
 * Vim-style keyboard navigation between sidebar chats.
 *
 * - Ctrl+J / Cmd+J: Select the next chat.
 * - Ctrl+K / Cmd+K: Select the previous chat.
 * - Ctrl+Shift+J / Cmd+Shift+J: Select the last chat.
 * - Ctrl+Shift+K / Cmd+Shift+K: Select the first chat.
 * - Escape while a sidebar chat row has focus: Focus the composer.
 *
 * `visibleChatIds` must match the sidebar's visual order. `allChatIds`
 * is the same order including chats hidden by collapsed sections or
 * parents; it anchors the cursor when the active chat is hidden so
 * next/previous resolve to the nearest visible neighbor. When the
 * active chat is in neither list, next selects the first chat and
 * previous selects the last chat. Keys are ignored while focus is
 * inside a dialog.
 */
export function useChatVimNavigation({
	enabled,
	visibleChatIds,
	allChatIds,
	activeChatId,
	onSelectChat,
}: {
	enabled: boolean;
	visibleChatIds: readonly string[];
	allChatIds: readonly string[];
	activeChatId: string | undefined;
	onSelectChat: (chatId: string) => void;
}) {
	useEffect(() => {
		if (!enabled) {
			return;
		}

		const handler = (event: KeyboardEvent) => {
			const active = document.activeElement;
			if (active instanceof HTMLElement && active.closest(DIALOG_SELECTOR)) {
				return;
			}

			if (event.key === "Escape") {
				if (
					!(active instanceof HTMLElement) ||
					!active.closest(CHAT_ROW_SELECTOR)
				) {
					return;
				}
				const composer = document.querySelector(COMPOSER_SELECTOR);
				if (composer instanceof HTMLElement) {
					event.preventDefault();
					composer.focus();
				}
				return;
			}

			const isModifierPressed = isMac() ? event.metaKey : event.ctrlKey;
			if (!isModifierPressed || event.altKey) {
				return;
			}

			const isNext = isLetterKey(event, "j");
			const isPrevious = isLetterKey(event, "k");
			if (!isNext && !isPrevious) {
				return;
			}
			if (visibleChatIds.length === 0) {
				return;
			}
			event.preventDefault();

			const forward = isNext;
			const targetId = event.shiftKey
				? visibleChatIds[forward ? visibleChatIds.length - 1 : 0]
				: findNeighbor({ visibleChatIds, allChatIds, activeChatId, forward });
			if (targetId === undefined || targetId === activeChatId) {
				return;
			}
			onSelectChat(targetId);
		};

		document.addEventListener("keydown", handler);
		return () => document.removeEventListener("keydown", handler);
	}, [enabled, visibleChatIds, allChatIds, activeChatId, onSelectChat]);
}

function findNeighbor({
	visibleChatIds,
	allChatIds,
	activeChatId,
	forward,
}: {
	visibleChatIds: readonly string[];
	allChatIds: readonly string[];
	activeChatId: string | undefined;
	forward: boolean;
}): string | undefined {
	const lastIndex = visibleChatIds.length - 1;
	if (activeChatId === undefined) {
		return visibleChatIds[forward ? 0 : lastIndex];
	}

	const visibleIndex = visibleChatIds.indexOf(activeChatId);
	if (visibleIndex >= 0) {
		const next = forward
			? Math.min(visibleIndex + 1, lastIndex)
			: Math.max(visibleIndex - 1, 0);
		return visibleChatIds[next];
	}

	// The active chat is hidden. Walk outward from its position in
	// the full order to the nearest visible chat in the requested
	// direction, then clamp at the list edge.
	const hiddenIndex = allChatIds.indexOf(activeChatId);
	if (hiddenIndex < 0) {
		return visibleChatIds[forward ? 0 : lastIndex];
	}
	const visible = new Set(visibleChatIds);
	const step = forward ? 1 : -1;
	for (let i = hiddenIndex + step; i >= 0 && i < allChatIds.length; i += step) {
		const id = allChatIds[i];
		if (id !== undefined && visible.has(id)) {
			return id;
		}
	}
	return visibleChatIds[forward ? lastIndex : 0];
}

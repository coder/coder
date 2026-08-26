/**
 * Storage keys shared by multiple AgentsPage modules, plus the
 * per-chat key families that archive and delete flows clean up.
 * Keys with a single consumer are defined next to that consumer.
 *
 * Existing key names and serialized value formats are preserved so
 * upgrading (or rolling back) does not lose users' stored
 * preferences.
 */

import {
	booleanCodec,
	defineEntityStorageKey,
	defineStorageKey,
	integerCodec,
	jsonCodec,
	stringCodec,
} from "#/storage";
import { chatDraftAttachmentsStorage } from "./utils/chatDraftAttachmentStorage";

export const chatFullWidthStorage = defineStorageKey<boolean>({
	key: "agents.chat-full-width",
	codec: booleanCodec,
	defaultValue: false,
});

export const rightPanelOpenStorage = defineStorageKey<boolean>({
	key: "agents.right-panel-open",
	codec: booleanCodec,
	defaultValue: false,
});

export const rightPanelWidthStorage = defineStorageKey<number | null>({
	key: "agents.right-panel-width",
	codec: integerCodec,
	defaultValue: null,
});

/**
 * Draft for the create-chat form. The value is either serialized
 * Lexical editor state or a legacy plain-text draft; parseStoredDraft
 * tells them apart.
 */
export const emptyInputDraftStorage = defineStorageKey<string | null>({
	key: "agents.empty-input",
	codec: stringCodec,
	defaultValue: null,
});

export const lastModelConfigIdStorage = defineStorageKey<string | null>({
	key: "agents.last-model-config-id",
	codec: stringCodec,
	defaultValue: null,
});

export const modelConfigReasoningEffortStorage = defineEntityStorageKey<
	string | null
>({
	prefix: "agents.reasoning-effort.",
	codec: stringCodec,
	defaultValue: null,
});

/**
 * Chat input draft: serialized Lexical editor state or a legacy
 * plain-text draft, distinguished by parseStoredDraft.
 */
export const chatDraftInputStorage = defineEntityStorageKey<string | null>({
	prefix: "agents.draft-input.",
	codec: stringCodec,
	defaultValue: null,
});

export const chatSidebarTabStorage = defineEntityStorageKey<string | null>({
	prefix: "agents.last-active-tab.",
	codec: stringCodec,
	defaultValue: null,
});

const emptyTabs: readonly unknown[] = [];

const parseJsonArray = (parsed: unknown): readonly unknown[] | undefined =>
	Array.isArray(parsed) ? parsed : undefined;

/**
 * Stored as raw tab descriptors; callers narrow entries with
 * isUserRightPanelTab so stale shapes from older builds are dropped
 * on read.
 */
export const chatRightPanelTabsStorage = defineEntityStorageKey<
	readonly unknown[]
>({
	prefix: "agents.right-panel-tabs.",
	codec: jsonCodec<readonly unknown[]>(parseJsonArray),
	defaultValue: emptyTabs,
});

export const chatDefaultTerminalHiddenStorage = defineEntityStorageKey<boolean>(
	{
		prefix: "agents.default-terminal-hidden.",
		codec: booleanCodec,
		defaultValue: false,
	},
);

/**
 * Remove every per-chat key for the given chat. Wire this into every
 * mutation that archives or deletes a chat so per-chat keys cannot
 * leak.
 */
export function clearChatStorage(chatId: string): void {
	chatDraftInputStorage.clear(chatId);
	chatSidebarTabStorage.clear(chatId);
	chatRightPanelTabsStorage.clear(chatId);
	chatDefaultTerminalHiddenStorage.clear(chatId);
	chatDraftAttachmentsStorage.clear(chatId);
}

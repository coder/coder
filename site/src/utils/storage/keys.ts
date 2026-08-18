/**
 * Registry of every browser storage key used by the app: the single
 * source of truth for key names, value codecs, defaults, and entity
 * lifecycles. Import handles from here; do not touch localStorage
 * directly at call sites.
 *
 * Existing key names and serialized value formats are preserved so
 * upgrading (or rolling back) does not lose users' stored
 * preferences. Entity-scoped values additionally gain a companion
 * timestamp key so orphans can be expired by the sweep.
 */

import type {
	Region,
	WorkspaceAgentPortShareProtocol,
} from "#/api/typesGenerated";
import {
	booleanCodec,
	defineEntityStorageKey,
	defineStorageKey,
	integerCodec,
	jsonCodec,
	registerLegacyStorageKeys,
	type StorageCodec,
	type SweepAction,
	stringCodec,
	stringLiteralCodec,
} from "./storage";

export { clearEntityStorage, sweepExpiredStorage } from "./storage";

const dayMs = 24 * 60 * 60 * 1000;
/** Drafts match the pre-existing 30 day attachment sweep window. */
const draftTtlMs = 30 * dayMs;
/** Per-entity UI preferences are kept longer than drafts. */
const entityPreferenceTtlMs = 90 * dayMs;

// -- Global preferences -----------------------------------------------------

export const chatFullWidthStorage = defineStorageKey<boolean>({
	key: "agents.chat-full-width",
	codec: booleanCodec,
	defaultValue: false,
});

export const leftSidebarWidthStorage = defineStorageKey<number | null>({
	key: "agents.left-sidebar-width",
	codec: integerCodec,
	defaultValue: null,
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

export const chimeOnCompletionStorage = defineStorageKey<boolean>({
	key: "agents.chime-on-completion",
	codec: booleanCodec,
	defaultValue: false,
});

export type DiffViewStyle = "unified" | "split";

export const diffViewStyleStorage = defineStorageKey<DiffViewStyle>({
	key: "agents.diff-view-style",
	codec: stringLiteralCodec<DiffViewStyle>(["unified", "split"]),
	defaultValue: "unified",
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

export const selectedOrganizationIdStorage = defineStorageKey<string | null>({
	key: "agents.selected-organization-id",
	codec: stringCodec,
	defaultValue: null,
});

export const selectedWorkspaceIdStorage = defineStorageKey<string | null>({
	key: "agents.selected-workspace-id",
	codec: stringCodec,
	defaultValue: null,
});

export const lastModelConfigIdStorage = defineStorageKey<string | null>({
	key: "agents.last-model-config-id",
	codec: stringCodec,
	defaultValue: null,
});

/**
 * Metadata for already-uploaded create-form attachments so they
 * survive page navigations without re-uploading.
 */
type PersistedChatAttachment = {
	fileId: string;
	fileName: string;
	fileType: string;
	lastModified: number;
	organizationId: string;
};

const isPersistedChatAttachment = (
	item: unknown,
): item is PersistedChatAttachment => {
	if (typeof item !== "object" || item === null) {
		return false;
	}
	const record = item as Record<string, unknown>;
	return (
		typeof record.fileId === "string" &&
		typeof record.fileName === "string" &&
		typeof record.fileType === "string" &&
		typeof record.lastModified === "number" &&
		typeof record.organizationId === "string"
	);
};

// Filters entry-by-entry so one legacy or corrupt record (for example
// pre-org-scoping data) does not discard valid siblings.
const isPersistedChatAttachments = (
	parsed: unknown,
): PersistedChatAttachment[] | undefined =>
	Array.isArray(parsed) ? parsed.filter(isPersistedChatAttachment) : undefined;

export const persistedAttachmentsStorage = defineStorageKey<
	PersistedChatAttachment[] | null
>({
	key: "agents.persisted-attachments",
	codec: jsonCodec(isPersistedChatAttachments),
	defaultValue: null,
});

type VSCodeVariant = "vscode" | "vscode-insiders";

export const vscodeVariantStorage = defineStorageKey<VSCodeVariant>({
	key: "vscode-variant",
	codec: stringLiteralCodec<VSCodeVariant>(["vscode", "vscode-insiders"]),
	defaultValue: "vscode",
});

export const dismissedUpdateVersionStorage = defineStorageKey<string | null>({
	key: "dismissedVersion",
	codec: stringCodec,
	defaultValue: null,
});

/** Timestamp of the last chunk-preload-failure reload (see index.tsx). */
export const preloadReloadStorage = defineStorageKey<number | null>({
	key: "preload-reload",
	codec: integerCodec,
	defaultValue: null,
	area: "session",
});

const isRegion = (parsed: unknown): Region | undefined => {
	if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
		return undefined;
	}
	const record = parsed as Record<string, unknown>;
	// Rebuild from the validated fields rather than asserting the
	// stored object, so extra or stale properties never leak through.
	if (
		typeof record.id !== "string" ||
		typeof record.name !== "string" ||
		typeof record.display_name !== "string" ||
		typeof record.icon_url !== "string" ||
		typeof record.healthy !== "boolean" ||
		typeof record.path_app_url !== "string" ||
		typeof record.wildcard_hostname !== "string"
	) {
		return undefined;
	}
	return {
		id: record.id,
		name: record.name,
		display_name: record.display_name,
		icon_url: record.icon_url,
		healthy: record.healthy,
		path_app_url: record.path_app_url,
		wildcard_hostname: record.wildcard_hostname,
	};
};

export const userSelectedProxyStorage = defineStorageKey<Region | null>({
	key: "user-selected-proxy",
	codec: jsonCodec(isRegion),
	defaultValue: null,
});

/** Mirrors ProxyLatencyReport in contexts/useProxyLatency.ts. */
type StoredProxyLatencyReport = {
	accurate: boolean;
	latencyMS: number;
	at: Date;
	nextHopProtocol?: string;
};

type StoredProxyLatencies = Record<string, StoredProxyLatencyReport[]>;

/**
 * Latencies persist `at` as an ISO string, so decoding revives it to
 * a Date the way the previous hand-rolled reviver did.
 */
const isStoredProxyLatencyReport = (
	entry: unknown,
): entry is StoredProxyLatencyReport => {
	if (typeof entry !== "object" || entry === null) {
		return false;
	}
	const record = entry as Record<string, unknown>;
	return (
		typeof record.accurate === "boolean" &&
		typeof record.latencyMS === "number" &&
		Number.isFinite(record.latencyMS) &&
		record.at instanceof Date &&
		!Number.isNaN(record.at.getTime()) &&
		(record.nextHopProtocol === undefined ||
			typeof record.nextHopProtocol === "string")
	);
};

const proxyLatenciesCodec: StorageCodec<StoredProxyLatencies> = {
	decode: (raw) => {
		try {
			const parsed: unknown = JSON.parse(raw, (key, value) =>
				key === "at" ? new Date(value) : value,
			);
			if (
				typeof parsed !== "object" ||
				parsed === null ||
				Array.isArray(parsed)
			) {
				return undefined;
			}
			// Drop malformed entries instead of failing the whole cache;
			// each report must be fully valid before it can feed the
			// latency-based proxy auto-selection.
			const validated: StoredProxyLatencies = {};
			for (const [proxyId, reports] of Object.entries(parsed)) {
				if (Array.isArray(reports)) {
					validated[proxyId] = reports.filter(isStoredProxyLatencyReport);
				}
			}
			return validated;
		} catch {
			return undefined;
		}
	},
	encode: (value) => JSON.stringify(value),
};

/**
 * Cached per-proxy latency reports so a single slow request does not
 * dominate the displayed latency.
 */
export const workspaceProxyLatenciesStorage =
	defineStorageKey<StoredProxyLatencies | null>({
		key: "workspace-proxy-latencies",
		codec: proxyLatenciesCodec,
		defaultValue: null,
	});

/** Dev knob: how many latency reports to keep per proxy (default 1). */
export const workspaceProxyLatenciesMaxStorage = defineStorageKey<
	number | null
>({
	key: "workspace-proxy-latencies-max",
	codec: integerCodec,
	defaultValue: null,
});

// -- Per-chat keys ----------------------------------------------------------

/**
 * Chat input draft: serialized Lexical editor state or a legacy
 * plain-text draft, distinguished by parseStoredDraft.
 */
export const chatDraftInputStorage = defineEntityStorageKey<string | null>({
	prefix: "agents.draft-input.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: draftTtlMs,
});

export const chatSidebarTabStorage = defineEntityStorageKey<string | null>({
	prefix: "agents.last-active-tab.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: entityPreferenceTtlMs,
});

const emptyTabs: readonly unknown[] = [];

const isJsonArray = (parsed: unknown): readonly unknown[] | undefined =>
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
	entity: "chat",
	codec: jsonCodec<readonly unknown[]>(isJsonArray),
	defaultValue: emptyTabs,
	ttlMs: entityPreferenceTtlMs,
});

export const chatDefaultTerminalHiddenStorage = defineEntityStorageKey<boolean>(
	{
		prefix: "agents.default-terminal-hidden.",
		entity: "chat",
		codec: booleanCodec,
		defaultValue: false,
		ttlMs: entityPreferenceTtlMs,
	},
);

// -- Chat draft attachments (own record format) ------------------------------

const isRecordObject = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

/**
 * Per-record prune preserving the pre-existing behavior of
 * pruneExpiredChatDraftAttachmentStorageKeys: records carry their own
 * updatedAt, so expire records individually instead of whole values.
 */
const sweepChatDraftAttachments = (raw: string, nowMs: number): SweepAction => {
	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch {
		return "remove";
	}
	if (!Array.isArray(parsed)) {
		return "remove";
	}
	const activeRecords = parsed.filter((entry) => {
		if (!isRecordObject(entry) || typeof entry.updatedAt !== "number") {
			return true;
		}
		return nowMs - entry.updatedAt <= draftTtlMs;
	});
	if (activeRecords.length === 0) {
		return "remove";
	}
	if (activeRecords.length !== parsed.length) {
		return { rewrite: JSON.stringify(activeRecords) };
	}
	return "keep";
};

/**
 * Draft attachment records for a chat, keyed by organization and chat
 * ID (`forId(organizationId, chatId)`). Records carry their own
 * timestamps, so the family needs no companion timestamp key; domain
 * logic lives in chatDraftAttachmentStorage.ts and works with the raw
 * JSON string.
 */
export const chatDraftAttachmentsStorage = defineEntityStorageKey<
	string | null
>({
	prefix: "agents.chat-draft-attachments.",
	entity: "chat",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: draftTtlMs,
	timestamped: false,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
	sweepValue: sweepChatDraftAttachments,
});

// -- Per-model and per-workspace keys ----------------------------------------

export const modelConfigReasoningEffortStorage = defineEntityStorageKey<
	string | null
>({
	prefix: "agents.reasoning-effort.",
	entity: "modelConfig",
	codec: stringCodec,
	defaultValue: null,
	ttlMs: entityPreferenceTtlMs,
});

export const workspaceListeningPortsProtocolStorage =
	defineEntityStorageKey<WorkspaceAgentPortShareProtocol>({
		prefix: "listening-ports-protocol-workspace-",
		entity: "workspace",
		codec: stringLiteralCodec<WorkspaceAgentPortShareProtocol>([
			"http",
			"https",
		]),
		defaultValue: "http",
		ttlMs: entityPreferenceTtlMs,
	});

// -- Legacy keys ------------------------------------------------------------

// Impending-deletion dismissals that are no longer written anywhere;
// the sweep removes them unconditionally.
registerLegacyStorageKeys(["dismissedWorkspaceList", "dismissedWorkspace"]);

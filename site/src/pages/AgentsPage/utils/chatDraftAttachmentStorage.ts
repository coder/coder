import { defineEntityStorageKey, stringCodec } from "#/storage";
import { decodeDataURL } from "./dataUrls";

/** Matches the pre-existing 30 day draft retention window. */
const maxStoredDraftAgeMs = 30 * 24 * 60 * 60 * 1000;

/**
 * Draft attachment records for a chat, keyed by organization and chat
 * ID (`forId(organizationId, chatId)`). Records carry their own
 * timestamps and expire on read after 30 days.
 */
export const chatDraftAttachmentsStorage = defineEntityStorageKey<
	string | null
>({
	prefix: "agents.chat-draft-attachments.",
	codec: stringCodec,
	defaultValue: null,
	entityIdFromSuffix: (suffix) => suffix.split(".").at(-1) ?? suffix,
});

type ChatDraftAttachmentRecord = {
	clientId: string;
	fileName: string;
	fileType: string;
	lastModified: number;
	size: number;
	updatedAt?: number;
	organizationId: string;
	chatId: string;
} & (
	| {
			status: "pending" | "uploading";
			payload: string;
	  }
	| {
			status: "uploaded";
			fileId: string;
	  }
);

export type RestoredChatDraftAttachment = {
	record: ChatDraftAttachmentRecord;
	file: File;
};

type ChatDraftAttachmentPersistResult =
	| { ok: true }
	| { ok: false; reason: "quota" | "unavailable" };

export const chatDraftAttachmentStorageKey = (
	organizationId: string,
	chatId: string,
) => chatDraftAttachmentsStorage.forId(organizationId, chatId).key;

const isRecordObject = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const isString = (value: unknown): value is string => typeof value === "string";

const isFiniteNumber = (value: unknown): value is number =>
	typeof value === "number" && Number.isFinite(value);

const validateRecord = (
	value: unknown,
	organizationId: string,
	chatId: string,
): ChatDraftAttachmentRecord | null => {
	if (!isRecordObject(value)) {
		return null;
	}
	const {
		clientId,
		fileName,
		fileType,
		lastModified,
		size,
		updatedAt,
		organizationId: recordOrganizationId,
		chatId: recordChatId,
		status,
	} = value;
	if (
		!isString(clientId) ||
		!isString(fileName) ||
		!isString(fileType) ||
		!isFiniteNumber(lastModified) ||
		!isFiniteNumber(size) ||
		!isString(recordOrganizationId) ||
		!isString(recordChatId) ||
		recordOrganizationId !== organizationId ||
		recordChatId !== chatId
	) {
		return null;
	}
	const recordUpdatedAt = isFiniteNumber(updatedAt) ? updatedAt : Date.now();
	if (Date.now() - recordUpdatedAt > maxStoredDraftAgeMs) {
		return null;
	}
	if (status === "pending" || status === "uploading") {
		const { payload } = value;
		if (!isString(payload)) {
			return null;
		}
		return {
			status,
			clientId,
			fileName,
			fileType,
			lastModified,
			size,
			updatedAt: recordUpdatedAt,
			organizationId: recordOrganizationId,
			chatId: recordChatId,
			payload,
		};
	}
	if (status === "uploaded") {
		const { fileId } = value;
		if (!isString(fileId)) {
			return null;
		}
		return {
			status,
			clientId,
			fileId,
			fileName,
			fileType,
			lastModified,
			size,
			updatedAt: recordUpdatedAt,
			organizationId: recordOrganizationId,
			chatId: recordChatId,
		};
	}
	return null;
};

const dedupeRecords = (
	records: readonly ChatDraftAttachmentRecord[],
): ChatDraftAttachmentRecord[] => {
	const byClientId = new Map<string, ChatDraftAttachmentRecord>();
	for (const record of records) {
		byClientId.set(record.clientId, record);
	}
	const byFileId = new Set<string>();
	const deduped: ChatDraftAttachmentRecord[] = [];
	for (const record of byClientId.values()) {
		if (record.status === "uploaded") {
			if (byFileId.has(record.fileId)) {
				continue;
			}
			byFileId.add(record.fileId);
		}
		deduped.push(record);
	}
	return deduped;
};

const writeRecords = (
	organizationId: string,
	chatId: string,
	records: readonly ChatDraftAttachmentRecord[],
): ChatDraftAttachmentPersistResult => {
	const handle = chatDraftAttachmentsStorage.forId(organizationId, chatId);
	const deduped = dedupeRecords(records);
	if (deduped.length === 0) {
		handle.remove();
		return { ok: true };
	}
	return handle.set(JSON.stringify(deduped));
};

const readRecords = (
	organizationId: string,
	chatId: string,
): ChatDraftAttachmentRecord[] => {
	const handle = chatDraftAttachmentsStorage.forId(organizationId, chatId);
	const stored = handle.get();
	if (!stored) {
		return [];
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(stored);
	} catch {
		handle.remove();
		return [];
	}
	if (!Array.isArray(parsed)) {
		handle.remove();
		return [];
	}
	const records = parsed.flatMap((entry) => {
		const record = validateRecord(entry, organizationId, chatId);
		return record ? [record] : [];
	});
	const deduped = dedupeRecords(records);
	if (deduped.length !== parsed.length) {
		writeRecords(organizationId, chatId, deduped);
	}
	return deduped;
};

const getRecordMetadata = (record: ChatDraftAttachmentRecord) => ({
	fileName: record.fileName,
	fileType: record.fileType,
	lastModified: record.lastModified,
});

export const fileToDataURL = (file: File): Promise<string> =>
	new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onerror = () =>
			reject(reader.error ?? new Error("Failed to read file."));
		reader.onload = () => {
			if (typeof reader.result === "string") {
				resolve(reader.result);
				return;
			}
			reject(new Error("Failed to read file."));
		};
		reader.readAsDataURL(file);
	});

const fileFromDataURL = (
	payload: string,
	metadata: { fileName: string; fileType: string; lastModified: number },
): File | null => {
	const decoded = decodeDataURL(payload);
	// FileReader stores drafts as base64, so other encodings indicate corruption.
	if (!decoded?.isBase64) {
		return null;
	}
	if (
		metadata.fileType &&
		decoded.mediaType &&
		decoded.mediaType.toLowerCase() !== metadata.fileType.toLowerCase()
	) {
		return null;
	}
	return new File([decoded.bytes], metadata.fileName, {
		type: metadata.fileType,
		lastModified: metadata.lastModified,
	});
};

const fileForRecord = (record: ChatDraftAttachmentRecord): File | null => {
	if (record.status === "uploaded") {
		return new File([], record.fileName, {
			type: record.fileType,
			lastModified: record.lastModified,
		});
	}
	return fileFromDataURL(record.payload, getRecordMetadata(record));
};

// Drop expired records from every chat's stored drafts, not just the
// active chat: orphaned drafts (for example for chats archived from
// another client) would otherwise hold base64 payloads forever.
const pruneExpiredChatDraftAttachments = () => {
	const now = Date.now();
	for (const suffix of chatDraftAttachmentsStorage.listStoredSuffixes()) {
		const handle = chatDraftAttachmentsStorage.forId(suffix);
		const stored = handle.get();
		if (!stored) {
			continue;
		}
		let parsed: unknown;
		try {
			parsed = JSON.parse(stored);
		} catch {
			handle.remove();
			continue;
		}
		if (!Array.isArray(parsed)) {
			handle.remove();
			continue;
		}
		const activeRecords = parsed.filter((entry) => {
			if (!isRecordObject(entry) || !isFiniteNumber(entry.updatedAt)) {
				return true;
			}
			return now - entry.updatedAt <= maxStoredDraftAgeMs;
		});
		if (activeRecords.length === 0) {
			handle.remove();
		} else if (activeRecords.length !== parsed.length) {
			handle.set(JSON.stringify(activeRecords));
		}
	}
};

export const restoreChatDraftAttachments = (
	organizationId: string | undefined,
	chatId: string | undefined,
): RestoredChatDraftAttachment[] => {
	if (!organizationId || !chatId) {
		return [];
	}
	pruneExpiredChatDraftAttachments();
	const restored: RestoredChatDraftAttachment[] = [];
	const validRecords: ChatDraftAttachmentRecord[] = [];
	for (const record of readRecords(organizationId, chatId)) {
		const file = fileForRecord(record);
		if (!file) {
			continue;
		}
		restored.push({ record, file });
		validRecords.push(record);
	}
	writeRecords(organizationId, chatId, validRecords);
	return restored;
};

export const upsertChatDraftAttachmentRecord = (
	record: ChatDraftAttachmentRecord,
): ChatDraftAttachmentPersistResult => {
	const recordWithTimestamp = { ...record, updatedAt: Date.now() };
	const records = readRecords(record.organizationId, record.chatId).filter(
		(existing) => {
			if (existing.clientId === record.clientId) {
				return false;
			}
			return !(
				existing.status === "uploaded" &&
				recordWithTimestamp.status === "uploaded" &&
				existing.fileId === recordWithTimestamp.fileId
			);
		},
	);
	return writeRecords(record.organizationId, record.chatId, [
		...records,
		recordWithTimestamp,
	]);
};

export const removeChatDraftAttachmentRecord = (
	organizationId: string,
	chatId: string,
	clientId: string,
): ChatDraftAttachmentPersistResult => {
	const records = readRecords(organizationId, chatId).filter(
		(record) => record.clientId !== clientId,
	);
	return writeRecords(organizationId, chatId, records);
};

export const clearChatDraftAttachmentRecords = (
	organizationId: string,
	chatId: string,
): ChatDraftAttachmentPersistResult => writeRecords(organizationId, chatId, []);

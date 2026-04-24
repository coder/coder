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

const storageKeyPrefix = "agents.chat-draft-attachments";
const maxStoredDraftAgeMs = 30 * 24 * 60 * 60 * 1000;

export const chatDraftAttachmentStorageKey = (
	organizationId: string,
	chatId: string,
) => `${storageKeyPrefix}.${organizationId}.${chatId}`;

const isRecordObject = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const isString = (value: unknown): value is string => typeof value === "string";

const isFiniteNumber = (value: unknown): value is number =>
	typeof value === "number" && Number.isFinite(value);

const isQuotaError = (error: unknown): boolean => {
	if (!(error instanceof DOMException)) {
		return false;
	}
	return (
		error.name === "QuotaExceededError" ||
		error.name === "NS_ERROR_DOM_QUOTA_REACHED"
	);
};

const safeSetItem = (
	key: string,
	value: string,
): ChatDraftAttachmentPersistResult => {
	try {
		localStorage.setItem(key, value);
		return { ok: true };
	} catch (error) {
		return { ok: false, reason: isQuotaError(error) ? "quota" : "unavailable" };
	}
};

const safeRemoveItem = (key: string) => {
	try {
		localStorage.removeItem(key);
	} catch {
		// Ignore storage cleanup failures. The in-memory draft remains usable.
	}
};

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
	const key = chatDraftAttachmentStorageKey(organizationId, chatId);
	const deduped = dedupeRecords(records);
	if (deduped.length === 0) {
		safeRemoveItem(key);
		return { ok: true };
	}
	return safeSetItem(key, JSON.stringify(deduped));
};

const pruneExpiredChatDraftAttachmentStorageKeys = () => {
	const now = Date.now();
	try {
		for (let index = localStorage.length - 1; index >= 0; index--) {
			const key = localStorage.key(index);
			if (!key?.startsWith(`${storageKeyPrefix}.`)) {
				continue;
			}
			const stored = localStorage.getItem(key);
			if (!stored) {
				continue;
			}
			let parsed: unknown;
			try {
				parsed = JSON.parse(stored);
			} catch {
				continue;
			}
			if (!Array.isArray(parsed)) {
				continue;
			}
			const activeRecords = parsed.filter((entry) => {
				if (!isRecordObject(entry) || !isFiniteNumber(entry.updatedAt)) {
					return true;
				}
				return now - entry.updatedAt <= maxStoredDraftAgeMs;
			});
			if (activeRecords.length === 0) {
				safeRemoveItem(key);
			} else if (activeRecords.length !== parsed.length) {
				safeSetItem(key, JSON.stringify(activeRecords));
			}
		}
	} catch {
		// Ignore storage sweep failures. The active chat restore still runs below.
	}
};

const readRecords = (
	organizationId: string,
	chatId: string,
): ChatDraftAttachmentRecord[] => {
	const key = chatDraftAttachmentStorageKey(organizationId, chatId);
	let stored: string | null = null;
	try {
		stored = localStorage.getItem(key);
	} catch {
		return [];
	}
	if (!stored) {
		return [];
	}
	let parsed: unknown;
	try {
		parsed = JSON.parse(stored);
	} catch {
		safeRemoveItem(key);
		return [];
	}
	if (!Array.isArray(parsed)) {
		safeRemoveItem(key);
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
	const commaIndex = payload.indexOf(",");
	if (commaIndex === -1 || !payload.startsWith("data:")) {
		return null;
	}
	const header = payload.slice(0, commaIndex);
	if (!header.toLowerCase().includes(";base64")) {
		return null;
	}
	const payloadMediaType = header.slice("data:".length).split(";")[0];
	if (
		metadata.fileType &&
		payloadMediaType &&
		payloadMediaType.toLowerCase() !== metadata.fileType.toLowerCase()
	) {
		return null;
	}
	try {
		const binary = atob(payload.slice(commaIndex + 1));
		const bytes = new Uint8Array(binary.length);
		for (let index = 0; index < binary.length; index++) {
			bytes[index] = binary.charCodeAt(index);
		}
		return new File([bytes], metadata.fileName, {
			type: metadata.fileType,
			lastModified: metadata.lastModified,
		});
	} catch {
		return null;
	}
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

export const restoreChatDraftAttachments = (
	organizationId: string | undefined,
	chatId: string | undefined,
): RestoredChatDraftAttachment[] => {
	if (!organizationId || !chatId) {
		return [];
	}
	pruneExpiredChatDraftAttachmentStorageKeys();
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

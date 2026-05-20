const ATTACHMENT_FALLBACK_EXTENSIONS: Record<string, string> = {
	"application/json": "json",
	"application/octet-stream": "bin",
	"application/pdf": "pdf",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		"pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": "xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		"docx",
	"application/x-tar": "tar",
	"image/jpeg": "jpg",
	"text/markdown": "md",
	"text/plain": "txt",
};

const sanitizeAttachmentExtension = (value: string): string => {
	const sanitized = value
		.replace(/[^a-z0-9]/gi, "")
		.slice(0, 4)
		.toLowerCase();
	return sanitized || "file";
};

export const getFileAttachmentExtension = (attachment: {
	mediaType?: string;
	name?: string;
}): string => {
	const mediaType = attachment.mediaType?.trim() ?? "";
	const mapped = ATTACHMENT_FALLBACK_EXTENSIONS[mediaType];
	if (mapped) {
		return mapped;
	}
	const trimmedName = attachment.name?.trim();
	if (trimmedName) {
		const lastDot = trimmedName.lastIndexOf(".");
		if (lastDot > 0 && lastDot < trimmedName.length - 1) {
			return sanitizeAttachmentExtension(trimmedName.slice(lastDot + 1));
		}
	}
	const subtype = mediaType.split("/")[1] ?? "";
	if (subtype.endsWith("+json")) {
		return "json";
	}
	return sanitizeAttachmentExtension(subtype);
};

export const getFileAttachmentBadgeLabel = (attachment: {
	mediaType?: string;
	name?: string;
}): string => {
	const extension = getFileAttachmentExtension(attachment);
	return extension === "file" ? "" : extension.toUpperCase();
};

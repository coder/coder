const IMAGE_ATTACHMENT_PREFIX = "image/";
export const MAX_CHAT_IMAGE_ATTACHMENTS = 4;
const MAX_CHAT_IMAGE_ATTACHMENT_BYTES = 5 * 1024 * 1024;
const IMAGE_ATTACHMENT_LIMIT_MB =
	MAX_CHAT_IMAGE_ATTACHMENT_BYTES / (1024 * 1024);

const describeAttachment = (file: Pick<File, "name">): string => {
	const trimmedName = file.name.trim();
	return trimmedName.length > 0 ? trimmedName : "This attachment";
};

const isImageAttachment = (file: Pick<File, "type">): boolean =>
	file.type.startsWith(IMAGE_ATTACHMENT_PREFIX);

export const validateImageAttachment = (
	file: Pick<File, "name" | "size" | "type">,
): string | undefined => {
	if (!isImageAttachment(file)) {
		return `${describeAttachment(file)} is not a supported image file.`;
	}
	if (!Number.isFinite(file.size) || file.size < 0) {
		return `${describeAttachment(file)} has an invalid file size.`;
	}
	if (file.size > MAX_CHAT_IMAGE_ATTACHMENT_BYTES) {
		return `${describeAttachment(file)} exceeds the ${IMAGE_ATTACHMENT_LIMIT_MB} MB attachment limit.`;
	}
	return undefined;
};

export const readFileAsDataURL = async (file: File): Promise<string> => {
	const validationError = validateImageAttachment(file);
	if (validationError) {
		throw new Error(validationError);
	}
	if (typeof FileReader !== "function") {
		throw new Error("File uploads are unavailable in this browser.");
	}

	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onerror = () => {
			reject(
				reader.error ??
					new Error(`Failed to read ${describeAttachment(file)}.`),
			);
		};
		reader.onload = () => {
			const result = reader.result;
			if (typeof result !== "string" || !result.startsWith("data:")) {
				reject(
					new Error(
						`Failed to encode ${describeAttachment(file)} as an image attachment.`,
					),
				);
				return;
			}
			resolve(result);
		};
		reader.readAsDataURL(file);
	});
};

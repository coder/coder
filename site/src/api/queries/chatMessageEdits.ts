import type * as TypesGen from "#/api/typesGenerated";

const buildOptimisticEditedContent = ({
	requestContent,
	originalMessage,
	attachmentMediaTypes,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	attachmentMediaTypes?: ReadonlyMap<string, string>;
}): readonly TypesGen.ChatMessagePart[] => {
	const existingFilePartsByID = new Map<string, TypesGen.ChatFilePart>();
	for (const part of originalMessage.content ?? []) {
		if (part.type === "file" && part.file_id) {
			existingFilePartsByID.set(part.file_id, part);
		}
	}

	return requestContent.map((part): TypesGen.ChatMessagePart => {
		if (part.type === "text") {
			return { type: "text", text: part.text ?? "" };
		}
		if (part.type === "file-reference") {
			return {
				type: "file-reference",
				file_name: part.file_name ?? "",
				start_line: part.start_line ?? 1,
				end_line: part.end_line ?? 1,
				content: part.content ?? "",
			};
		}
		if (part.type === "workspace-file-reference") {
			return {
				type: "workspace-file-reference",
				workspace_file_path: part.workspace_file_path ?? "",
				workspace_file_name: part.workspace_file_name ?? "",
				workspace_file_size: part.workspace_file_size ?? 0,
				workspace_file_media_type: part.workspace_file_media_type,
				workspace_file_workspace_id: part.workspace_file_workspace_id ?? "",
			};
		}
		const fileId = part.file_id ?? "";
		return (
			existingFilePartsByID.get(fileId) ?? {
				type: "file",
				file_id: part.file_id,
				media_type:
					attachmentMediaTypes?.get(fileId) ?? "application/octet-stream",
			}
		);
	});
};

export const buildOptimisticEditedMessage = ({
	requestContent,
	originalMessage,
	attachmentMediaTypes,
}: {
	requestContent: readonly TypesGen.ChatInputPart[];
	originalMessage: TypesGen.ChatMessage;
	attachmentMediaTypes?: ReadonlyMap<string, string>;
}): TypesGen.ChatMessage => ({
	...originalMessage,
	content: buildOptimisticEditedContent({
		requestContent,
		originalMessage,
		attachmentMediaTypes,
	}),
});

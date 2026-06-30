import type * as TypesGen from "#/api/typesGenerated";

export type BuiltInSlashCommandID = "compact";

export type BuiltInSlashCommand = {
	id: BuiltInSlashCommandID;
	trigger: string;
	name: string;
	description: string;
};

export const builtInSlashCommands: readonly BuiltInSlashCommand[] = [
	{
		id: "compact",
		trigger: "/compact",
		name: "compact",
		description: "Summarize earlier chat history without sending a message.",
	},
];

export type BuiltInSlashCommandParseInput = {
	message: string;
	content: readonly TypesGen.ChatInputPart[];
	isEditing?: boolean;
	hasAttachments?: boolean;
};

export const parseBuiltInSlashCommand = ({
	message,
	content,
	isEditing,
	hasAttachments,
}: BuiltInSlashCommandParseInput): BuiltInSlashCommandID | null => {
	if (isEditing || hasAttachments || message.trim() !== "/compact") {
		return null;
	}
	if (content.some((part) => part.type !== "text")) {
		return null;
	}
	return "compact";
};

export const filterBuiltInSlashCommands = (
	query: string,
): readonly BuiltInSlashCommand[] => {
	const normalizedQuery = query.toLocaleLowerCase("en-US");
	if (!normalizedQuery) {
		return builtInSlashCommands;
	}
	return builtInSlashCommands.filter(
		(command) =>
			command.name.startsWith(normalizedQuery) ||
			command.description.toLocaleLowerCase("en-US").includes(normalizedQuery),
	);
};

/**
 * A command offered by the "/" trigger menu. Selecting one inserts
 * "/name" into the composer; inputHint is placeholder text for the
 * arguments a command accepts after its name.
 */
export type ChatComposerCommand = {
	name: string;
	description: string;
	inputHint?: string;
};

/**
 * A built-in chat command. Unlike personal skills and runtime commands,
 * built-ins are fixed client-side actions: the composer intercepts them
 * at submit time instead of sending the text as a message.
 */
type ChatSlashCommand = ChatComposerCommand & {
	name: "clear" | "compact";
};

export const COMPACT_SLASH_COMMAND: ChatSlashCommand = {
	name: "compact",
	description:
		"Summarize the conversation so far to free up context window space",
};

export const CLEAR_SLASH_COMMAND: ChatSlashCommand = {
	name: "clear",
	description: "Clear the conversation context; the next message starts fresh",
};

/**
 * Commands available in the main chat composer. Editing an existing
 * message and the new-agent form do not offer commands.
 */
export const CHAT_SLASH_COMMANDS: readonly ChatSlashCommand[] = [
	COMPACT_SLASH_COMMAND,
	CLEAR_SLASH_COMMAND,
];

type ChatSlashCommandResolution = "pending" | "available" | "unavailable";

export const resolveChatSlashCommandAvailability = (
	command: ChatSlashCommand,
	personalSkills: readonly { name: string }[] | undefined,
	workspaceSkills: readonly { name: string }[] | undefined,
): ChatSlashCommandResolution => {
	if (personalSkills === undefined || workspaceSkills === undefined) {
		return "pending";
	}
	return [...personalSkills, ...workspaceSkills].some(
		(skill) => skill.name === command.name,
	)
		? "unavailable"
		: "available";
};

export const chatSlashCommandTriggerText = (
	command: ChatSlashCommand,
): string => `/${command.name}`;

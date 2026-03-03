import {
	BotIcon,
	FileIcon,
	FilePenIcon,
	PlusCircleIcon,
	TerminalIcon,
	WrenchIcon,
} from "lucide-react";
import type React from "react";
import { cn } from "utils/cn";

export const ToolIcon: React.FC<{ name: string; isError: boolean }> = ({
	name,
	isError,
}) => {
	const color = isError ? "text-content-destructive" : "text-content-secondary";
	const base = cn("h-4 w-4 shrink-0", color);
	switch (name) {
		case "execute":
			return <TerminalIcon className={base} />;
		case "read_file":
		case "list_templates":
		case "read_template":
			return <FileIcon className={base} />;
		case "write_file":
		case "edit_files":
			return <FilePenIcon className={base} />;
		case "create_workspace":
			return <PlusCircleIcon className={base} />;
		case "chat_summarized":
			return <BotIcon className={base} />;
		default:
			return <WrenchIcon className={base} />;
	}
};

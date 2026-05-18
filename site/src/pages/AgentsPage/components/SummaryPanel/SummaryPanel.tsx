import { FileTextIcon } from "lucide-react";
import type { FC } from "react";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";

interface SummaryPanelProps {
	chatTitle: string | undefined;
}

export const SummaryPanel: FC<SummaryPanelProps> = ({ chatTitle }) => {
	return (
		<ScrollArea className="flex h-full flex-col">
			<div className="flex flex-col items-center justify-center gap-3 p-6 text-center text-content-secondary">
				<FileTextIcon className="size-8 text-content-tertiary" />
				<h3 className="text-sm font-medium text-content-primary">
					{chatTitle ?? "Chat Summary"}
				</h3>
				<p className="max-w-xs text-xs">
					A summary of this chat session will appear here as the conversation
					progresses.
				</p>
			</div>
		</ScrollArea>
	);
};

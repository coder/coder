import { HistoryIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { scrollToUserSentinelAfterClose } from "./scrollToUserSentinel";

export interface PromptHistoryEntry {
	/** The message ID used to locate the sentinel in the DOM. */
	id: number;
	/** 1-based index of this prompt in the conversation. */
	index: number;
	/** Display label for the user message (may include synthetic attachment labels). */
	label: string;
}

interface PromptHistoryPopoverProps {
	entries: readonly PromptHistoryEntry[];
	onSelectEntry?: (entry: PromptHistoryEntry) => void | Promise<void>;
	/** Called when the popover opens/closes so the parent action bar
	 *  can stay visible while the dropdown is active. */
	onOpenChange?: (open: boolean) => void;
}

/**
 * A popover that lists every user prompt in the conversation. Clicking an
 * entry scrolls the chat so that prompt becomes visible.
 */
export const PromptHistoryPopover: FC<PromptHistoryPopoverProps> = ({
	entries,
	onSelectEntry,
	onOpenChange,
}) => {
	const [open, setOpen] = useState(false);

	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		onOpenChange?.(next);
	};

	if (entries.length < 2) {
		return null;
	}

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<Tooltip>
				<TooltipTrigger asChild>
					<PopoverTrigger asChild>
						<Button
							size="icon"
							variant="subtle"
							className="size-6"
							aria-label="Prompt history"
						>
							<HistoryIcon />
						</Button>
					</PopoverTrigger>
				</TooltipTrigger>
				<TooltipContent side="bottom">Prompt history</TooltipContent>
			</Tooltip>

			<PopoverContent
				align="end"
				side="bottom"
				className="w-[calc(100vw-2rem)] overflow-hidden p-0 sm:w-80"
			>
				<Command loop>
					<CommandInput placeholder="Search prompts..." />
					<CommandList>
						<CommandEmpty>No matching prompts</CommandEmpty>
						<CommandGroup>
							{entries.map((entry) => (
								<CommandItem
									key={entry.id}
									value={`${entry.index} ${entry.label}`}
									onSelect={() => {
										handleOpenChange(false);
										if (onSelectEntry) {
											void onSelectEntry(entry);
											return;
										}
										scrollToUserSentinelAfterClose(entry.id);
									}}
								>
									<span className="min-w-[1.5rem] text-sm tabular-nums text-content-disabled">
										{entry.index}
									</span>
									<span className="min-w-0 truncate text-sm text-content-primary">
										{entry.label || "Empty message"}
									</span>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

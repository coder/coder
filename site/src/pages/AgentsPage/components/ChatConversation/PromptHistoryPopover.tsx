import { HistoryIcon, LoaderCircleIcon } from "lucide-react";
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
	onSelectEntry?: (entry: PromptHistoryEntry) => unknown;
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
	const [loadingEntryId, setLoadingEntryId] = useState<number | null>(null);

	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		onOpenChange?.(next);
	};

	const handleSelectEntry = (entry: PromptHistoryEntry) => {
		if (loadingEntryId !== null) {
			return;
		}

		void (async () => {
			setLoadingEntryId(entry.id);
			const result = await onSelectEntry?.(entry);
			setLoadingEntryId(null);
			if (result === false) {
				return;
			}
			handleOpenChange(false);
			scrollToUserSentinelAfterClose(entry.id);
		})();
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
				<Command loop aria-busy={loadingEntryId !== null || undefined}>
					<CommandInput
						placeholder="Search prompts..."
						disabled={loadingEntryId !== null}
					/>
					<CommandList>
						<CommandEmpty>No matching prompts</CommandEmpty>
						<CommandGroup>
							{entries.map((entry) => (
								<CommandItem
									key={entry.id}
									value={`${entry.index} ${entry.label}`}
									disabled={loadingEntryId !== null}
									onSelect={() => handleSelectEntry(entry)}
								>
									<span className="min-w-[1.5rem] text-sm tabular-nums text-content-disabled">
										{entry.index}
									</span>
									<span className="min-w-0 flex-1 truncate text-sm text-content-primary">
										{entry.label || "Empty message"}
									</span>
									{loadingEntryId === entry.id && (
										<LoaderCircleIcon className="size-3 animate-spin text-content-secondary" />
									)}
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
					{loadingEntryId !== null && (
						<div
							role="status"
							className="flex items-center gap-2 border-0 border-t border-solid border-border px-3 py-2 text-xs text-content-secondary"
						>
							<LoaderCircleIcon className="size-3 animate-spin" />
							Loading prompt...
						</div>
					)}
				</Command>
			</PopoverContent>
		</Popover>
	);
};

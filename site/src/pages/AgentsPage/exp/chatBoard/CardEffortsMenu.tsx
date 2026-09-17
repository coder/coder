import type { FC, ReactNode } from "react";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import { InlineEdit } from "./InlineEdit";

interface CardEffortsMenuProps {
	readonly open: boolean;
	readonly onOpenChange: (open: boolean) => void;
	readonly selected: readonly string[];
	/** Every effort on the board, offered as checkboxes. */
	readonly known: readonly string[];
	readonly onChange: (names: readonly string[]) => void;
	/** The header meta the editor drops below; the actions menu inside it opens the editor. */
	readonly children: ReactNode;
}

/**
 * Checkboxes for every effort on the board plus a line to coin a new one;
 * each change saves. Anchored to the card meta so it drops below the header.
 */
export const CardEffortsMenu: FC<CardEffortsMenuProps> = ({
	open,
	onOpenChange,
	selected,
	known,
	onChange,
	children,
}) => {
	const options = [...new Set([...known, ...selected])];
	const toggle = (name: string, on: boolean) =>
		onChange(
			on ? [...selected, name] : selected.filter((other) => other !== name),
		);
	return (
		<Popover open={open} onOpenChange={onOpenChange}>
			<PopoverAnchor asChild>{children}</PopoverAnchor>
			<PopoverContent
				side="bottom"
				align="end"
				sideOffset={6}
				className="flex w-48 flex-col gap-1 p-2 text-xs"
				onPointerDown={(e) => e.stopPropagation()}
				// The closing menu hands focus back to its trigger, which would
				// count as leaving the popover the moment it opens.
				onFocusOutside={(e) => e.preventDefault()}
			>
				{options.map((name) => (
					<label
						key={name}
						className="flex cursor-pointer items-center gap-2 text-content-primary"
					>
						<input
							type="checkbox"
							className="size-3 accent-content-link"
							checked={selected.includes(name)}
							onChange={(e) => toggle(name, e.target.checked)}
						/>
						<span className="truncate">{name}</span>
					</label>
				))}
				{/* Keyed by the count so the field clears after a new effort is coined. */}
				<InlineEdit
					key={selected.length}
					value=""
					placeholder="New effort"
					ariaLabel="New effort"
					className="mt-1 text-xs text-content-primary"
					onSave={(name) => toggle(name, true)}
					onDone={() => undefined}
				/>
			</PopoverContent>
		</Popover>
	);
};

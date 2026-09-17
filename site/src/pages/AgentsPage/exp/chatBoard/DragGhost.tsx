import { cn } from "cn";
import type { FC } from "react";
import type { DragData } from "./BoardCard";
import { cardAccent } from "./cardColor";

interface DragGhostProps {
	readonly drag: DragData;
}

/** Compact stand-in rendered in the DragOverlay while a card, chat, column, or note moves. */
export const DragGhost: FC<DragGhostProps> = ({ drag }) => {
	if (drag.type === "note") {
		return (
			<div className="w-[276px] cursor-grabbing truncate rounded-md border border-content-link bg-surface-primary px-3 py-1.5 text-xs text-content-primary shadow-lg">
				{drag.note.text}
			</div>
		);
	}
	if (drag.type === "column") {
		return (
			<div className="w-[300px] cursor-grabbing rounded-md border border-content-link bg-surface-primary px-3 py-1.5 text-[13px] font-medium text-content-primary shadow-lg">
				{drag.name}
			</div>
		);
	}
	const title = drag.type === "card" ? drag.card.title : drag.chat.title;
	const detail =
		drag.type === "card" && drag.card.members.length > 1
			? `${drag.card.members.length} chats`
			: undefined;
	return (
		<div
			className={cn(
				"w-[300px] cursor-grabbing rounded-lg border border-content-link bg-surface-primary px-3 py-2 text-sm shadow-lg",
				drag.type === "card" && cardAccent({ color: drag.card.color }),
			)}
		>
			<div className="font-medium leading-snug text-content-primary">
				{title}
			</div>
			{detail && <div className="text-xs text-content-secondary">{detail}</div>}
		</div>
	);
};

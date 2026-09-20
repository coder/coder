import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { cn } from "cn";
import { GripVerticalIcon } from "lucide-react";
import type { CSSProperties, FC, ReactNode, Ref } from "react";
import { NavLink, useLocation } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { shortRelativeTime } from "#/utils/time";
import { normalizeLocationSearch } from "../locationSearch";
import { getChatDisplayConfig } from "./statusConfig";

interface FlatChatRowProps {
	readonly chat: Chat;
	readonly isActive: boolean;
	/** Rendered before the link; a drag handle for pinned rows. */
	readonly leading?: ReactNode;
	readonly containerRef?: Ref<HTMLDivElement>;
	readonly style?: CSSProperties;
	readonly className?: string;
}

/** Flat shortcut row used by the pinned and shared sections above the tree. */
export const FlatChatRow: FC<FlatChatRowProps> = ({
	chat,
	isActive,
	leading,
	containerRef,
	style,
	className,
}) => {
	const location = useLocation();
	const locationSearch = normalizeLocationSearch(location.search);
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
	} = getChatDisplayConfig(chat);

	return (
		<div
			ref={containerRef}
			style={style}
			className={cn(
				"group flex min-w-0 items-center gap-1 rounded-md pr-1.5 text-content-secondary",
				"[@media(hover:hover)]:hover:bg-surface-tertiary/50 [@media(hover:hover)]:hover:text-content-primary",
				"has-[[aria-current=page]]:bg-surface-quaternary/50 has-[[aria-current=page]]:text-content-primary",
				className,
			)}
		>
			{leading ?? <span aria-hidden="true" className="size-6 shrink-0" />}
			<NavLink
				to={{ pathname: `/agents/${chat.id}`, search: locationSearch }}
				className="flex min-h-7 min-w-0 flex-1 items-center gap-1.5 py-1 text-inherit no-underline"
			>
				<StatusIcon
					role="img"
					aria-label={statusLabel}
					className={cn("size-3.5 shrink-0", statusClassName)}
				/>
				<span
					className={cn(
						"truncate text-[13px] text-content-primary",
						!isActive &&
							"opacity-85 [@media(hover:hover)]:group-hover:opacity-100",
					)}
				>
					{chat.title}
				</span>
				{chat.has_unread && !isActive && (
					<span className="sr-only">(unread)</span>
				)}
			</NavLink>
			<span
				data-pixel="ignore"
				className="inline-block w-7 shrink-0 text-right text-xs text-content-secondary/50 tabular-nums"
			>
				{shortRelativeTime(chat.updated_at)}
			</span>
		</div>
	);
};

interface SortablePinnedChatRowProps {
	readonly chat: Chat;
	readonly isActive: boolean;
}

/**
 * Pinned row with a dedicated drag handle. dnd-kit's attributes and
 * listeners go on the handle only, so the row link keeps native Enter
 * navigation and the wrapper carries no button role.
 */
export const SortablePinnedChatRow: FC<SortablePinnedChatRowProps> = ({
	chat,
	isActive,
}) => {
	const {
		attributes,
		listeners,
		setNodeRef,
		setActivatorNodeRef,
		transform,
		transition,
		isDragging,
	} = useSortable({
		id: chat.id,
		animateLayoutChanges: () => false,
	});
	const adjustedTransform = transform
		? { ...transform, scaleX: 1, scaleY: 1 }
		: null;

	return (
		<FlatChatRow
			chat={chat}
			isActive={isActive}
			containerRef={setNodeRef}
			style={{
				transform: CSS.Transform.toString(adjustedTransform),
				transition: isDragging ? "opacity 200ms" : transition,
			}}
			className={cn(isDragging && "opacity-50")}
			leading={
				<Button
					ref={setActivatorNodeRef}
					variant="subtle"
					size="icon"
					className="size-6 min-w-0 shrink-0 cursor-grab touch-none p-0 text-content-secondary/60 hover:text-content-primary [&>svg]:size-3.5"
					aria-label={`Reorder ${chat.title}`}
					{...attributes}
					{...listeners}
				>
					<GripVerticalIcon />
				</Button>
			}
		/>
	);
};

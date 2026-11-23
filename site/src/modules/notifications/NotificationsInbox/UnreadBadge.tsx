import { cn } from "cn";

/** Returns the unread count as the badge displays it, capped at "99+". */
export const formatUnreadCount = (count: number): string =>
	count > 99 ? "99+" : String(count);

type UnreadBadgeProps = {
	count: number;
} & React.HTMLProps<HTMLSpanElement>;

export const UnreadBadge: React.FC<UnreadBadgeProps> = ({
	count,
	className,
	...props
}) => {
	return (
		<span
			className={cn([
				"[--unread-badge-size:18px] min-w-(--unread-badge-size) h-(--unread-badge-size)",
				"flex w-fit px-1 rounded text-2xs items-center justify-center",
				"bg-surface-sky text-highlight-sky",
				className,
			])}
			{...props}
		>
			{formatUnreadCount(count)}
		</span>
	);
};

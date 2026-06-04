import { BellIcon } from "lucide-react";
import { useRef } from "react";
import { Button, type ButtonProps } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { UnreadBadge } from "./UnreadBadge";

type InboxButtonProps = ButtonProps & {
	unreadCount: number;
};

export const InboxButton: React.FC<InboxButtonProps> = ({
	unreadCount,
	...props
}) => {
	const isOpen = unreadCount > 0;
	// Track whether the badge has ever been shown so we can skip
	// the exit animation on initial mount with zero notifications.
	const hasBeenOpen = useRef(false);
	if (isOpen) {
		hasBeenOpen.current = true;
	}

	return (
		<Button size="icon-lg" variant="outline" className="relative" {...props}>
			<BellIcon />
			{/* Badge stays mounted so the exit transition can play. */}
			{hasBeenOpen.current && (
				<span
					className={cn(
						"group/badge pointer-events-none",
						"[--unread-badge-size:18px] [--offset:calc(var(--unread-badge-size)/2)]",
						"absolute top-0 right-0 -mr-[--offset] -mt-[--offset]",
					)}
					data-open={isOpen}
					aria-hidden={!isOpen || undefined}
				>
					<UnreadBadge
						count={unreadCount}
						className={cn(
							"origin-center transition-[transform,opacity] duration-500",
							"[transition-timing-function:cubic-bezier(0.34,1.36,0.64,1)]",
							"group-data-[open=false]/badge:scale-0",
							"group-data-[open=false]/badge:opacity-0",
							"group-data-[open=false]/badge:duration-150",
							"group-data-[open=false]/badge:[transition-timing-function:cubic-bezier(0.4,0,0.2,1)]",
						)}
					/>
				</span>
			)}
		</Button>
	);
};

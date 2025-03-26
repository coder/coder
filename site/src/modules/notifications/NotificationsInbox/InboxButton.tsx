import { Button, type ButtonProps } from "components/Button/Button";
import { BellIcon } from "lucide-react";
import { forwardRef } from "react";
import { UnreadBadge } from "./UnreadBadge";
import { cn } from "utils/cn";

type InboxButtonProps = {
	unreadCount: number;
} & ButtonProps;

export const InboxButton = forwardRef<HTMLButtonElement, InboxButtonProps>(
	({ unreadCount, ...props }, ref) => {
		return (
			<Button
				size="icon-lg"
				variant="outline"
				className="relative"
				ref={ref}
				{...props}
			>
				<BellIcon />
				{unreadCount > 0 && (
					<UnreadBadge
						count={unreadCount}
						className={cn([
							"[--offset:calc(var(--unread-badge-size)/2)]",
							"absolute top-0 right-0 -mr-[--offset] -mt-[--offset]",
							"animate-in fade-in zoom-in duration-200",
						])}
					/>
				)}
			</Button>
		);
	},
);

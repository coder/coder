import { Button, type ButtonProps } from "components/Button/Button";
import { BellIcon } from "lucide-react";
import { forwardRef, type FC } from "react";
import { UnreadBadge } from "./UnreadBadge";

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
						className="absolute top-0 right-0 -translate-y-1/2 translate-x-1/2"
					/>
				)}
			</Button>
		);
	},
);

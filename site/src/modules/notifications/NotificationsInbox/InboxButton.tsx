import { BellIcon } from "lucide-react";
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
	return (
		<Button size="icon-lg" variant="outline" className="relative" {...props}>
			<BellIcon />
			{unreadCount > 0 && (
				<UnreadBadge
					count={unreadCount}
					className={cn(
						"[--offset:calc(var(--unread-badge-size)/2)]",
						"absolute top-0 right-0 -mr-[--offset] -mt-[--offset]",
						"origin-center animate-in zoom-in-0 fade-in-0 duration-500",
						"[animation-timing-function:cubic-bezier(0.34,1.36,0.64,1)]",
					)}
				/>
			)}
		</Button>
	);
};

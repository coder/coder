import { Button, type ButtonProps } from "components/Button/Button";
import { BellIcon } from "lucide-react";
import { cn } from "utils/cn";
import { UnreadBadge } from "./UnreadBadge";

type InboxButtonProps = ButtonProps & {
	unreadCount: number;
};

export const InboxButton: React.FC<InboxButtonProps> = ({
	unreadCount,
	"aria-label": ariaLabel = "Notifications",
	...props
}) => {
	return (
		<Button
			size="icon-lg"
			variant="outline"
			className="relative"
			aria-label={ariaLabel}
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
};

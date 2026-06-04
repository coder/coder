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
	const isOpen = unreadCount > 0;

	return (
		<Button size="icon-lg" variant="outline" className="relative" {...props}>
			<BellIcon />
			{/*
			 * Notification badge transition (transitions.dev pattern).
			 * The wrapper slides diagonally into position via animate-badge-slide-in.
			 * The inner dot pops with overshoot scale + blur via group-data variants.
			 * The badge stays mounted so the exit transition can play.
			 */}
			<span
				className={cn(
					"group/badge pointer-events-none will-change-transform",
					"[--offset:calc(var(--unread-badge-size)/2)]",
					"absolute top-0 right-0 -mr-[--offset] -mt-[--offset]",
					isOpen && "animate-badge-slide-in",
				)}
				data-open={isOpen}
				aria-hidden={!isOpen || undefined}
			>
				<UnreadBadge
					count={unreadCount}
					className={cn(
						// Open state: pop in with bouncy overshoot easing.
						"origin-center transition-[transform,opacity,filter]",
						"[transition-duration:var(--badge-pop-dur),var(--badge-fade-dur),var(--badge-pop-dur)]",
						"[transition-timing-function:var(--badge-pop-ease)]",
						"will-change-[transform,opacity,filter]",
						// Close state: fast snappy exit with scale-to-zero + blur.
						"group-data-[open=false]/badge:scale-0",
						"group-data-[open=false]/badge:opacity-0",
						"group-data-[open=false]/badge:blur-[var(--badge-blur)]",
						"group-data-[open=false]/badge:[transition-duration:var(--badge-pop-close-dur),var(--badge-fade-close-dur),var(--badge-pop-close-dur)]",
						"group-data-[open=false]/badge:[transition-timing-function:var(--badge-close-ease)]",
					)}
				/>
			</span>
		</Button>
	);
};

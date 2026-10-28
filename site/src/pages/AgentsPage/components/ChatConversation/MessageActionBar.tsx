import { cn } from "cn";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type MessageActionBarProps = React.PropsWithChildren<{
	className?: string;
}>;

/**
 * Message rows and collapsed working blocks share it, so a turn's pill sits
 * at the same offset under either.
 */
export const MessageActionBar: React.FC<MessageActionBarProps> = ({
	className,
	children,
}) => (
	<div className={cn("mt-0.5 flex items-center gap-1", className)}>
		{children}
	</div>
);

type MessageActionButtonProps = {
	label: string;
	icon: React.ReactNode;
	onClick: () => void;
	disabled?: boolean;
};

export const MessageActionButton: React.FC<MessageActionButtonProps> = ({
	label,
	icon,
	onClick,
	disabled,
}) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<Button
				size="icon"
				variant="subtle"
				className="size-6"
				aria-label={label}
				disabled={disabled}
				onClick={onClick}
			>
				{icon}
			</Button>
		</TooltipTrigger>
		<TooltipContent side="bottom">{label}</TooltipContent>
	</Tooltip>
);

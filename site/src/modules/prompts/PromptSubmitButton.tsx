import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowUpIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

type PromptSubmitButtonProps = {
	disabled?: boolean;
	type?: "submit" | "button";
	onClick?: () => void;
	isLoading?: boolean;
	tooltip?: ReactNode;
	icon?: ReactNode;
	label: string;
};

export const PromptSubmitButton: FC<PromptSubmitButtonProps> = ({
	disabled,
	type = "submit",
	onClick,
	isLoading,
	tooltip,
	icon,
	label,
}) => {
	const button = (
		<Button
			size="icon"
			type={type}
			onClick={onClick}
			disabled={disabled}
			className="rounded-full disabled:bg-surface-invert-primary disabled:opacity-70"
		>
			<Spinner loading={isLoading}>{icon ?? <ArrowUpIcon />}</Spinner>
			<span className="sr-only">{label}</span>
		</Button>
	);

	if (tooltip) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>{button}</TooltipTrigger>
				<TooltipContent align="end">{tooltip}</TooltipContent>
			</Tooltip>
		);
	}

	return button;
};

import { CopyIcon } from "lucide-react";
import type { FC } from "react";
import { CheckIcon } from "#/components/AnimatedIcons/Check";
import { Button, type ButtonProps } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClipboard } from "#/hooks/useClipboard";

type CopyButtonProps = ButtonProps & {
	text: string;
	label: string;
	tooltipSide?: "top" | "bottom" | "left" | "right";
};

export const CopyButton: FC<CopyButtonProps> = ({
	text,
	label,
	tooltipSide,
	...buttonProps
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard();

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					size="icon"
					variant="subtle"
					onClick={() => copyToClipboard(text)}
					{...buttonProps}
				>
					{showCopiedSuccess ? <CheckIcon /> : <CopyIcon />}
					<span className="sr-only">{label}</span>
				</Button>
			</TooltipTrigger>
			<TooltipContent side={tooltipSide}>{label}</TooltipContent>
		</Tooltip>
	);
};

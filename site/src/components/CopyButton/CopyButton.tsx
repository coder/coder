import { CopyIcon } from "lucide-react";
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

export const CopyButton: React.FC<CopyButtonProps> = ({
	text,
	label,
	tooltipSide,
	...buttonProps
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard();

	return (
		<>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="subtle"
						aria-label={label}
						onClick={() => copyToClipboard(text)}
						{...buttonProps}
					>
						{showCopiedSuccess ? <CheckIcon /> : <CopyIcon />}
					</Button>
				</TooltipTrigger>
				<TooltipContent side={tooltipSide}>{label}</TooltipContent>
			</Tooltip>
			<CopiedStatus show={showCopiedSuccess} />
		</>
	);
};

type CopiedStatusProps = {
	show: boolean;
};

/**
 * Announces a successful copy to screen readers. Render it alongside the copy
 * control before the copy happens, so the live region already exists when its
 * text changes.
 */
export const CopiedStatus: React.FC<CopiedStatusProps> = ({ show }) => (
	<span role="status" className="sr-only">
		{show && "Copied"}
	</span>
);

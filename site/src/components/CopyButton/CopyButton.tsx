import { Button, type ButtonProps } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useClipboard } from "hooks/useClipboard";
import { CheckIcon, CopyIcon } from "lucide-react";
import type { FC } from "react";

type CopyButtonProps = ButtonProps & {
	text: string;
	label: string;
};

export const CopyButton: FC<CopyButtonProps> = ({
	text,
	label,
	...buttonProps
}) => {
	const { showCopiedSuccess, copyToClipboard } = useClipboard({
		textToCopy: text,
	});

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						size="icon"
						variant="subtle"
						onClick={copyToClipboard}
						{...buttonProps}
					>
						{showCopiedSuccess ? <CheckIcon /> : <CopyIcon />}
						<span className="sr-only">{label}</span>
					</Button>
				</TooltipTrigger>
				<TooltipContent>{label}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

import { cn } from "cn";
import { EyeIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { MarkdownPreviewState } from "./diffViewerItems";

export interface MarkdownPreviewControl extends MarkdownPreviewState {
	onToggle: () => void;
}

const PREVIEW_TOGGLE_LABEL = "Preview Markdown";

/**
 * Press toggle shown in a Markdown file's diff header. A disabled reason
 * keeps the button in the tab order with `aria-disabled` so the tooltip
 * explaining it is reachable by keyboard.
 */
export const MarkdownPreviewToggle: FC<MarkdownPreviewControl> = ({
	isRendered,
	disabledReason,
	onToggle,
}) => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={PREVIEW_TOGGLE_LABEL}
					aria-pressed={isRendered}
					aria-disabled={disabledReason ? true : undefined}
					onClick={() => {
						if (!disabledReason) {
							onToggle();
						}
					}}
					className={cn(
						"size-6 rounded-sm [&>svg]:size-3.5 [&>svg]:p-0",
						isRendered &&
							"bg-surface-quaternary text-content-primary hover:bg-surface-quaternary",
						disabledReason &&
							"cursor-default opacity-50 hover:text-content-secondary",
					)}
				>
					<EyeIcon />
				</Button>
			</TooltipTrigger>
			<TooltipContent side="bottom">
				{disabledReason ?? PREVIEW_TOGGLE_LABEL}
			</TooltipContent>
		</Tooltip>
	);
};

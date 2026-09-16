import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

export const IdpUnseenClaimWarning: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					aria-label="Unknown claim value"
					className="inline-flex items-center justify-center border-0 bg-transparent p-0 text-content-warning cursor-pointer"
				>
					<TriangleAlertIcon className="size-icon-xs" />
				</button>
			</TooltipTrigger>
			<TooltipContent
				align="start"
				alignOffset={-8}
				sideOffset={8}
				className="p-2 text-xs text-content-secondary max-w-sm"
			>
				This value has not be seen in the specified claim field before. You
				might want to check your IdP configuration and ensure that this value is
				not misspelled.
			</TooltipContent>
		</Tooltip>
	);
};

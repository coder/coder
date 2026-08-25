import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type CoderWorkspacesProductCardProps = {
	userLimitActual?: number;
	userLimitLimit?: number;
};

export const CoderWorkspacesProductCard: FC<
	CoderWorkspacesProductCardProps
> = ({ userLimitActual, userLimitLimit }) => {
	const actualLabel =
		userLimitActual === undefined
			? "\u2014"
			: userLimitActual.toLocaleString("en-US");
	const limitLabel = userLimitLimit
		? userLimitLimit.toLocaleString("en-US")
		: "Unlimited";

	return (
		<div className="rounded-sm border border-solid border-border px-6 py-4">
			<div className="text-sm font-medium text-content-primary">
				Coder Workspaces
			</div>
			<div className="mt-3 text-xs">
				<div className="flex items-center gap-1 font-medium text-content-secondary">
					<span>Active seat usage</span>
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								aria-label="Active seat usage information"
								className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
							>
								<InfoIcon className="size-3" />
							</button>
						</TooltipTrigger>
						<TooltipContent side="top" className="max-w-xs">
							Only Active user accounts consume license seats. Dormant and
							suspended accounts don't count toward the total.
						</TooltipContent>
					</Tooltip>
				</div>
				<div className="mt-0.5 text-sm font-medium text-content-primary">
					{actualLabel} / {limitLabel}
				</div>
			</div>
		</div>
	);
};

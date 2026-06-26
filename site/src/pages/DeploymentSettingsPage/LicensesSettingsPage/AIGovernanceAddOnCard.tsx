import { InfoIcon } from "lucide-react";
import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type AIGovernanceAddOnCardProps = {
	title: string;
	unit: string;
	actual?: number;
	limit: number;
	isExceeded: boolean;
};

export const AIGovernanceAddOnCard: FC<AIGovernanceAddOnCardProps> = ({
	title,
	unit,
	actual,
	limit,
	isExceeded,
}) => {
	const actualLabel = actual === undefined ? "—" : actual.toLocaleString();

	return (
		<div
			className={`min-w-[320px] flex-1 rounded-sm border border-solid py-3 ${
				isExceeded ? "border-border-destructive" : "border-border"
			}`}
		>
			<div className="flex items-center gap-1 px-6 py-1.5">
				<div className="flex flex-1 items-center gap-1">
					<div className="flex items-center gap-6">
						<div className="flex items-center gap-1">
							<span className="overflow-hidden text-ellipsis whitespace-nowrap text-sm font-medium text-content-primary">
								{title}
							</span>
							<Tooltip>
								<TooltipTrigger asChild>
									<button
										type="button"
										aria-label="AI Governance seat information"
										className="m-0 inline-flex appearance-none border-0 bg-transparent p-0 text-content-secondary"
									>
										<InfoIcon className="size-3" />
									</button>
								</TooltipTrigger>
								<TooltipContent side="top">
									Seats consumed by users using AI Governance features.
								</TooltipContent>
							</Tooltip>
						</div>
						<Badge variant="magenta" size="sm">
							AI add-on
						</Badge>
					</div>

					<div className="min-w-[100px] flex-1 pl-8 pr-3">
						<div className="text-xs">
							<div className="font-medium text-content-secondary">{unit}</div>
							<div className="font-normal text-content-primary">
								<span
									className={
										isExceeded
											? "text-content-destructive"
											: "text-content-primary"
									}
								>
									{actualLabel}
								</span>{" "}
								/ {limit.toLocaleString()}
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

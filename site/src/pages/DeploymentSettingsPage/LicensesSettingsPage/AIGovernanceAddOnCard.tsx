import { Badge } from "components/Badge/Badge";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";
import type { FC } from "react";

type AIGovernanceAddOnCardProps = {
	title: string;
	unit: string;
	actual: number;
	limit: number;
	includedWithPremium: number;
	additionalPurchased: number;
};

export const AIGovernanceAddOnCard: FC<AIGovernanceAddOnCardProps> = ({
	title,
	unit,
	actual,
	limit,
	includedWithPremium,
	additionalPurchased,
}) => {
	return (
		<div className="min-w-[320px] flex-1 rounded-md border border-solid border-border py-3">
			<div className="flex items-center gap-1 px-6 py-1.5">
				<div className="flex flex-1 items-center gap-1">
					<div className="flex items-center gap-6">
						<div className="flex items-center gap-1">
							<span className="overflow-hidden text-ellipsis whitespace-nowrap text-sm font-medium text-content-primary">
								{title}
							</span>
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="inline-flex text-content-secondary">
										<InfoIcon className="size-3" />
									</span>
								</TooltipTrigger>
								<TooltipContent side="top">
									Seats consumed by users using AI Governance features.
								</TooltipContent>
							</Tooltip>
						</div>
						<Badge variant="magenta" size="sm" border="solid">
							AI add-on
						</Badge>
					</div>

					<div className="min-w-[100px] flex-1 pl-8 pr-3">
						<div className="flex items-center gap-8">
							<div className="text-xs">
								<div className="font-medium text-content-secondary">{unit}</div>
								<div className="font-normal text-content-primary">
									{actual.toLocaleString()} / {limit.toLocaleString()}
								</div>
							</div>

							<div className="text-xs font-normal">
								<div className="text-content-secondary">
									{includedWithPremium.toLocaleString()} Included with premium
								</div>
								{additionalPurchased > 0 ? (
									<div className="text-content-secondary">
										{additionalPurchased.toLocaleString()} additional seats
										purchased
									</div>
								) : (
									<div className="text-content-disabled">
										No additional seats purchased
									</div>
								)}
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

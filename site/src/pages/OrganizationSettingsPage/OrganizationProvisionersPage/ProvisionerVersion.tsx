import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";

export type ProvisionerVersionProps = {
	buildVersion: string | undefined;
	provisionerVersion: string;
};

export const ProvisionerVersion: FC<ProvisionerVersionProps> = ({
	provisionerVersion,
	buildVersion,
}) => {
	return provisionerVersion === buildVersion ? (
		<span className="text-xs font-medium text-content-secondary">
			Up to date
		</span>
	) : (
		<TooltipProvider>
			<Tooltip delayDuration={0}>
				<TooltipTrigger asChild>
					<StatusIndicator
						variant="warning"
						size="sm"
						className="cursor-pointer"
						tabIndex={0}
					>
						<TriangleAlertIcon className="size-icon-xs" />
						Outdated
					</StatusIndicator>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs">
					<p className="m-0">
						This provisioner is out of date. You may experience issues when
						using a provisioner version that doesn't match your Coder
						deployment. Please upgrade to a newer version.
					</p>
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

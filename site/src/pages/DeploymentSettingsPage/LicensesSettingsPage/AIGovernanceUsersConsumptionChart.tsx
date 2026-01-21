import type { Feature } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { Link } from "components/Link/Link";
import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import { docs } from "utils/docs";

interface AIGovernanceUsersConsumptionProps {
	aiGovernanceUserFeature?: Feature;
}

export const AIGovernanceUsersConsumption: FC<
	AIGovernanceUsersConsumptionProps
> = ({ aiGovernanceUserFeature }) => {
	// If no feature is provided or it's disabled, show disabled state
	if (!aiGovernanceUserFeature?.enabled) {
		return (
			<div className="min-h-60 flex items-center justify-center rounded-lg border border-solid p-12">
				<div className="flex flex-col gap-4 items-center justify-center">
					<div className="flex flex-col gap-2 items-center justify-center">
						<span className="text-base">Users with AI Governance Add-On</span>
						<span className="text-content-secondary text-center max-w-[464px] mt-2">
							AI Governance is not included in your current license. Contact{" "}
							<Link href="mailto:sales@coder.com">sales</Link> to upgrade your
							license and unlock this addon.
						</span>
					</div>
				</div>
			</div>
		);
	}

	const limit = aiGovernanceUserFeature.limit;

	if (limit === undefined || limit < 0) {
		return <ErrorAlert error="Invalid license usage limits" />;
	}

	return (
		<section className="border border-solid rounded">
			<div className="p-4">
				<Collapsible>
					<header className="flex flex-col gap-2 items-start">
						<h3 className="text-md m-0 font-medium">
							Users with AI Governance Add-On
						</h3>

						<CollapsibleTrigger asChild>
							<Button
								className={`
                  h-auto p-0 border-0 bg-transparent font-medium text-content-secondary
                  hover:bg-transparent hover:text-content-primary
                  [&[data-state=open]_svg]:rotate-90
                `}
							>
								<ChevronRightIcon />
								Learn more
							</Button>
						</CollapsibleTrigger>
					</header>

					<CollapsibleContent
						className={`
              pt-2 pl-7 pr-5 space-y-4 font-medium max-w-[720px]
              text-sm text-content-secondary
              [&_p]:m-0 [&_ul]:m-0 [&_ul]:p-0 [&_ul]:list-none
            `}
					>
						<p>
							Users using AI features like{" "}
							<Link
								href={docs("/ai-coder/ai-bridge")}
								target="_blank"
								rel="noreferrer"
							>
								AI Bridge
							</Link>
							,{" "}
							<Link
								href={docs("/ai-coder/boundary/agent-boundary")}
								target="_blank"
								rel="noreferrer"
							>
								Boundary
							</Link>
							, or{" "}
							<Link
								href={docs("/ai-coder/tasks")}
								target="_blank"
								rel="noreferrer"
							>
								Tasks
							</Link>
						</p>
					</CollapsibleContent>
				</Collapsible>
			</div>

			<div className="px-6 py-12 border-0 border-t border-solid">
				<div className="flex flex-col gap-4 text-center justify-center items-center">
					<div className="flex flex-col gap-2 text-center justify-center items-center">
						<div className="text-3xl font-bold">{limit?.toLocaleString()}</div>
						<div className="text-sm text-content-secondary">Users Entitled</div>
					</div>
					<div className="text-sm text-content-secondary">
						Additional analytics and measurements coming soon
					</div>
				</div>
			</div>
		</section>
	);
};

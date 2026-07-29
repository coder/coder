import kebabCase from "lodash/fp/kebabCase";
import { BellOffIcon, RotateCcwIcon } from "lucide-react";
import type { FC } from "react";
import type { HealthcheckReport, HealthSeverity } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Sidebar, SidebarNavItem } from "#/components/Sidebar";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { HealthIcon } from "#/pages/HealthPage/Content";
import { createDayString } from "#/utils/createDayString";

const healthSections = {
	derp: "DERP",
	access_url: "Access URL",
	websocket: "Websocket",
	database: "Database",
	workspace_proxy: "Workspace Proxy",
	provisioner_daemons: "Provisioner Daemons",
} as const;

type HealthSectionKey = keyof typeof healthSections;

interface HealthSidebarViewProps {
	healthStatus: HealthcheckReport;
	isRefreshing: boolean;
	onRefresh: () => void;
}

/**
 * Displays health status summary and navigation for each healthcheck
 * section. Severity icons and dismissed-state bells live inside the
 * nav item children.
 */
const HealthSidebarView: FC<HealthSidebarViewProps> = ({
	healthStatus,
	isRefreshing,
	onRefresh,
}) => {
	const hasWarnings = Object.keys(healthSections).some((key) => {
		const section = healthStatus[key as HealthSectionKey];
		return section.warnings && section.warnings.length > 0;
	});

	return (
		<Sidebar>
			<div className="flex flex-col gap-4 pl-3 pb-4 text-sm">
				<div>
					<div className="flex items-center justify-between">
						<HealthIcon size={32} severity={healthStatus.severity} />

						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									size="icon-lg"
									variant="subtle"
									disabled={isRefreshing}
									data-testid="healthcheck-refresh-button"
									onClick={onRefresh}
								>
									{isRefreshing ? (
										<Spinner size="sm" loading />
									) : (
										<RotateCcwIcon className="size-5" />
									)}
								</Button>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								Refresh health checks
							</TooltipContent>
						</Tooltip>
					</div>
					<div className="mt-4 font-medium">
						{healthStatus.healthy ? "Healthy" : "Unhealthy"}
					</div>
					<div className="text-content-secondary leading-normal">
						{healthStatus.healthy
							? hasWarnings
								? "All systems operational, but performance might be degraded"
								: "All systems operational"
							: "Some issues have been detected"}
					</div>
				</div>

				<div className="flex flex-col">
					<span className="font-medium">Last check</span>
					<span
						data-pixel="ignore"
						className="text-content-secondary leading-normal"
					>
						{createDayString(healthStatus.time)}
					</span>
				</div>

				<div className="flex flex-col">
					<span className="font-medium">Version</span>
					<span
						data-pixel="ignore"
						className="text-content-secondary leading-normal"
					>
						{healthStatus.coder_version}
					</span>
				</div>
			</div>

			<div className="flex flex-col gap-1">
				{Object.entries(healthSections)
					.sort(([a], [b]) => a.localeCompare(b))
					.map(([key, label]) => {
						const section = healthStatus[key as HealthSectionKey];

						return (
							<SidebarNavItem key={key} href={`/health/${kebabCase(key)}`}>
								<HealthIcon
									size={16}
									severity={section.severity as HealthSeverity}
								/>
								{label}
								{section.dismissed && (
									<BellOffIcon className="size-icon-sm ml-auto text-content-disabled" />
								)}
							</SidebarNavItem>
						);
					})}
			</div>
		</Sidebar>
	);
};

export default HealthSidebarView;

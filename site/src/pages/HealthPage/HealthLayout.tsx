import CircularProgress from "@mui/material/CircularProgress";
import IconButton from "@mui/material/IconButton";
import { health, refreshHealth } from "api/queries/debug";
import type { HealthSeverity } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import kebabCase from "lodash/fp/kebabCase";
import { BellOffIcon, RotateCcwIcon } from "lucide-react";
import { DashboardFullPage } from "modules/dashboard/DashboardLayout";
import { type FC, Suspense } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { NavLink, Outlet } from "react-router";
import { cn } from "utils/cn";
import { createDayString } from "utils/createDayString";
import { pageTitle } from "utils/page";
import { HealthIcon } from "./Content";

const linkStyles = {
	normal: `
		text-content-secondary border-none text-sm w-full flex items-center gap-3
		text-left h-9 px-6 cursor-pointer no-underline transition-colors
		hover:bg-surface-secondary hover:text-content-primary
	`,
	active: "bg-surface-secondary text-content-primary",
};

export const HealthLayout: FC = () => {
	const queryClient = useQueryClient();
	const {
		data: healthStatus,
		isLoading,
		error,
	} = useQuery({
		...health(),
		refetchInterval: 30_000,
	});
	const { mutate: forceRefresh, isPending: isRefreshing } = useMutation(
		refreshHealth(queryClient),
	);
	const sections = {
		derp: "DERP",
		access_url: "Access URL",
		websocket: "Websocket",
		database: "Database",
		workspace_proxy: "Workspace Proxy",
		provisioner_daemons: "Provisioner Daemons",
	} as const;
	const visibleSections = filterVisibleSections(sections);

	if (isLoading) {
		return (
			<div className="p-6">
				<Loader />
			</div>
		);
	}

	if (error || !healthStatus) {
		return (
			<div className="p-6">
				<ErrorAlert error={error} />
			</div>
		);
	}

	return (
		<>
			<title>{pageTitle("Health")}</title>

			<DashboardFullPage>
				<div className="flex basis-0 flex-1 overflow-hidden">
					<div className="w-64 shrink-0 text-sm border-0 border-solid border-r border-r-border">
						<div className="flex flex-col gap-4 p-6">
							<div>
								<div className="flex items-center justify-between">
									<HealthIcon size={32} severity={healthStatus.severity} />

									<Tooltip>
										<TooltipTrigger asChild>
											<IconButton
												size="small"
												disabled={isRefreshing}
												data-testid="healthcheck-refresh-button"
												onClick={() => {
													forceRefresh();
												}}
											>
												{isRefreshing ? (
													<CircularProgress size={16} />
												) : (
													<RotateCcwIcon className="size-5" />
												)}
											</IconButton>
										</TooltipTrigger>
										<TooltipContent side="bottom">
											Refresh health checks
										</TooltipContent>
									</Tooltip>
								</div>
								<div className="font-medium mt-4">
									{healthStatus.healthy ? "Healthy" : "Unhealthy"}
								</div>
								<div className="text-content-secondary line-height-[150%]">
									{healthStatus.healthy
										? Object.keys(visibleSections).some((key) => {
												const section =
													healthStatus[key as keyof typeof visibleSections];
												return section.warnings && section.warnings.length > 0;
											})
											? "All systems operational, but performance might be degraded"
											: "All systems operational"
										: "Some issues have been detected"}
								</div>
							</div>

							<div className="flex flex-col">
								<span className="font-medium">Last check</span>
								<span
									data-chromatic="ignore"
									className="text-content-secondary line-height-[150%]"
								>
									{createDayString(healthStatus.time)}
								</span>
							</div>

							<div className="flex flex-col">
								<span className="font-medium">Version</span>
								<span
									data-chromatic="ignore"
									className="text-content-secondary line-height-[150%]"
								>
									{healthStatus.coder_version}
								</span>
							</div>
						</div>

						<nav className="flex flex-col gap-px">
							{Object.entries(visibleSections)
								.sort()
								.map(([key, label]) => {
									const healthSection =
										healthStatus[key as keyof typeof visibleSections];

									return (
										<NavLink
											end
											key={key}
											to={`/health/${kebabCase(key)}`}
											className={({ isActive }) =>
												cn(linkStyles.normal, isActive && linkStyles.active)
											}
										>
											<HealthIcon
												size={16}
												severity={healthSection.severity as HealthSeverity}
											/>
											{label}
											{healthSection.dismissed && (
												<BellOffIcon className="size-icon-sm ml-auto text-content-disabled" />
											)}
										</NavLink>
									);
								})}
						</nav>
					</div>

					<div className="overflow-y-auto w-full">
						<Suspense fallback={<Loader />}>
							<Outlet context={healthStatus} />
						</Suspense>
					</div>
				</div>
			</DashboardFullPage>
		</>
	);
};

const filterVisibleSections = <T extends object>(sections: T) => {
	const visible: Partial<T> = {};

	for (const [sectionName, sectionValue] of Object.entries(sections)) {
		if (!sectionValue) {
			continue;
		}

		visible[sectionName as keyof T] = sectionValue;
	}

	return visible;
};

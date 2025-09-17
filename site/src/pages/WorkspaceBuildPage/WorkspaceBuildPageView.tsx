import type {
	ProvisionerJobLog,
	WorkspaceAgent,
	WorkspaceBuild,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
	FullWidthPageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/FullWidthPageHeader";
import { Stack } from "components/Stack/Stack";
import { Stats, StatsItem } from "components/Stats/Stats";
import { TAB_PADDING_X, TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { BuildAvatar } from "modules/builds/BuildAvatar/BuildAvatar";
import { DashboardFullPage } from "modules/dashboard/DashboardLayout";
import { AgentLogs } from "modules/resources/AgentLogs/AgentLogs";
import { useAgentLogs } from "modules/resources/useAgentLogs";
import {
	WorkspaceBuildData,
	WorkspaceBuildDataSkeleton,
} from "modules/workspaces/WorkspaceBuildData/WorkspaceBuildData";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
	type FC,
	type HTMLProps,
	type ReactNode,
	useLayoutEffect,
	useRef,
} from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import { displayWorkspaceBuildDuration } from "utils/workspace";
import { Sidebar, SidebarCaption, SidebarItem } from "./Sidebar";

export const LOGS_TAB_KEY = "logs";

type BuildStatsItemProps = Readonly<{
	children?: ReactNode;
	label: string;
}>;

const BuildStatsItem: FC<BuildStatsItemProps> = ({ children, label }) => {
	return (
		<StatsItem
			className="flex-col gap-0 p-0 [&>span:first-of-type]:text-xs [&>span:first-of-type]:font-medium md:p-0"
			label={label}
			value={children}
		/>
	);
};

interface WorkspaceBuildPageViewProps {
	logs: ProvisionerJobLog[] | undefined;
	build: WorkspaceBuild | undefined;
	buildError?: unknown;
	builds: WorkspaceBuild[] | undefined;
	activeBuildNumber: number;
}

export const WorkspaceBuildPageView: FC<WorkspaceBuildPageViewProps> = ({
	logs,
	build,
	buildError,
	builds,
	activeBuildNumber,
}) => {
	const tabState = useSearchParamsKey({
		key: LOGS_TAB_KEY,
		defaultValue: "build",
	});

	if (buildError) {
		return (
			<Margins>
				<ErrorAlert error={buildError} className="my-4" />
			</Margins>
		);
	}

	if (!build) {
		return <Loader />;
	}

	const agents = build.resources.flatMap((r) => r.agents ?? []);
	const selectedAgent = agents.find((a) => a.id === tabState.value);

	return (
		<DashboardFullPage>
			<FullWidthPageHeader sticky={false}>
				<Stack direction="row">
					<BuildAvatar build={build} size="lg" />
					<div>
						<PageHeaderTitle>Build #{build.build_number}</PageHeaderTitle>
						<PageHeaderSubtitle>{build.initiator_name}</PageHeaderSubtitle>
					</div>
				</Stack>

				<Stats
					aria-label="Build details"
					className="flex flex-col items-start gap-2 px-0 border-none grow basis-0 md:flex-row md:gap-x-12 md:gap-y-6"
				>
					<BuildStatsItem label="Workspace">
						<Link
							to={`/@${build.workspace_owner_name}/${build.workspace_name}`}
						>
							{build.workspace_name}
						</Link>
					</BuildStatsItem>
					<BuildStatsItem label="Template version">
						{build.template_version_name}
					</BuildStatsItem>
					<BuildStatsItem label="Duration">
						{displayWorkspaceBuildDuration(build)}
					</BuildStatsItem>
					<BuildStatsItem label="Started at">
						{new Date(build.created_at).toLocaleString()}
					</BuildStatsItem>
					<BuildStatsItem label="Action">
						<span className="capitalize">{build.transition}</span>
					</BuildStatsItem>
				</Stats>
			</FullWidthPageHeader>

			<div className="flex items-start overflow-hidden grow basis-0">
				<Sidebar>
					<SidebarCaption>Builds</SidebarCaption>
					{!builds &&
						Array.from({ length: 15 }, (_, i) => (
							<SidebarItem key={i}>
								<WorkspaceBuildDataSkeleton />
							</SidebarItem>
						))}

					{builds?.map((build) => (
						<Link
							key={build.id}
							to={`/@${build.workspace_owner_name}/${build.workspace_name}/builds/${build.build_number}`}
						>
							<SidebarItem active={build.build_number === activeBuildNumber}>
								<WorkspaceBuildData build={build} />
							</SidebarItem>
						</Link>
					))}
				</Sidebar>

				<ScrollArea>
					<Tabs active={tabState.value}>
						<TabsList className="gap-0">
							<TabLink
								to={`?${LOGS_TAB_KEY}=build`}
								value="build"
								className="px-6 pb-2"
							>
								Build
							</TabLink>

							{agents.map((a) => (
								<TabLink
									className="px-6 pb-2"
									to={`?${LOGS_TAB_KEY}=${a.id}`}
									value={a.id}
									key={a.id}
								>
									coder_agent.{a.name}
								</TabLink>
							))}
						</TabsList>
					</Tabs>
					{build.transition === "delete" && build.job.status === "failed" && (
						<Alert
							severity="error"
							className="rounded-none border-0 border-b border-solid border-border"
						>
							<div>
								The workspace may have failed to delete due to a Terraform state
								mismatch. A template admin may run{" "}
								<code className="font-semibold w-fit inline-block">
									{`coder rm ${`${build.workspace_owner_name}/${build.workspace_name}`} --orphan`}
								</code>{" "}
								to delete the workspace skipping resource destruction.
							</div>
						</Alert>
					)}

					{build?.job?.logs_overflowed && (
						<Alert
							severity="warning"
							className="rounded-none border-0 border-b border-solid border-border"
						>
							Provisioner logs exceeded the max size of 1MB. Will not continue
							to write provisioner logs for workspace build.
						</Alert>
					)}

					{tabState.value === "build" && (
						<BuildLogsContent logs={logs} build={build} />
					)}
					{tabState.value !== "build" && selectedAgent && (
						<AgentLogsContent agent={selectedAgent} />
					)}
				</ScrollArea>
			</div>
		</DashboardFullPage>
	);
};

const ScrollArea: FC<HTMLProps<HTMLDivElement>> = ({ className, ...props }) => {
	/**
	 * @todo 2024-10-03 - Use only CSS to set the height of the content.
	 *
	 * On Safari, when content is rendered inside a flex container and needs to
	 * scroll, the parent container must have a height set. Achieving this may
	 * require significant refactoring of the layout components where we
	 * currently use height and min-height set to 100%.
	 *
	 * @see {@link https://github.com/coder/coder/issues/9687}
	 * @see {@link https://stackoverflow.com/questions/43381836/height100-works-in-chrome-but-not-in-safari}
	 */
	const contentRef = useRef<HTMLDivElement>(null);
	useLayoutEffect(() => {
		const contentEl = contentRef.current;
		if (!contentEl) {
			return;
		}

		/**
		 * 2025-09-17 - We're updating the height directly to minimize the
		 * overhead in React itself. There is a risk down the line that the
		 * height value will be wiped on re-renders, but that seemed like a
		 * small enough risk that it wasn't worth accounting for just yet
		 */
		const syncParentSize = () => {
			const parentEl = contentEl.parentElement;
			if (parentEl && contentEl) {
				contentEl.style.height = `${contentEl.parentElement.clientHeight}px`;
			}
		};

		syncParentSize();
		const resizeObserver = new ResizeObserver(syncParentSize);
		resizeObserver.observe(document.body);
		return () => {
			resizeObserver.disconnect();
		};
	}, []);

	return (
		<div
			ref={contentRef}
			className={cn("overflow-y-auto w-full", className)}
			{...props}
		/>
	);
};

function sortLogsByCreatedAt(
	logs: readonly ProvisionerJobLog[],
): ProvisionerJobLog[] {
	return [...logs].sort((a, b) => {
		return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
	});
}

const BuildLogsContent: FC<{
	logs?: ProvisionerJobLog[];
	build?: WorkspaceBuild;
}> = ({ logs = [], build }) => {
	if (!logs) {
		return <Loader />;
	}

	return (
		<WorkspaceBuildLogs
			// logs header class adds extra spacing to the first log header to
			// prevent it from being too close to the tabs
			className="border-none [&_.logs-header:first-of-type]:pt-4"
			style={{ "--log-line-side-padding": `${TAB_PADDING_X}px` }}
			build={build}
			logs={sortLogsByCreatedAt(logs)}
			disableAutoscroll
		/>
	);
};

type AgentLogsContentProps = {
	agent: WorkspaceAgent;
};

const AgentLogsContent: FC<AgentLogsContentProps> = ({ agent }) => {
	const logs = useAgentLogs({ agentId: agent.id });
	return (
		<AgentLogs
			overflowed={agent.logs_overflowed}
			sources={agent.log_sources}
			height={560}
			width="100%"
			logs={logs.map((l) => ({
				id: l.id,
				output: l.output,
				time: l.created_at,
				level: l.level,
				sourceId: l.source_id,
			}))}
		/>
	);
};

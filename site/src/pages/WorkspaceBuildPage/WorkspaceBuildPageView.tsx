import { ExternalLinkIcon } from "lucide-react";
import {
	type FC,
	type HTMLProps,
	type ReactNode,
	useLayoutEffect,
	useRef,
} from "react";
import { Link, useSearchParams } from "react-router";
import type {
	ProvisionerJobLog,
	WorkspaceAgent,
	WorkspaceBuild,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import {
	FullWidthPageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/FullWidthPageHeader";
import { Stack } from "#/components/Stack/Stack";
import { Stats, StatsItem } from "#/components/Stats/Stats";
import {
	TAB_PADDING_X,
	Tabs,
	TabsContent,
	TabsList,
	TabsTrigger,
} from "#/components/Tabs/Tabs";
import { BuildAvatar } from "#/modules/builds/BuildAvatar/BuildAvatar";
import { DashboardFullPage } from "#/modules/dashboard/DashboardLayout";
import { AgentLogs } from "#/modules/resources/AgentLogs/AgentLogs";
import { useAgentLogs } from "#/modules/resources/useAgentLogs";
import {
	WorkspaceBuildData,
	WorkspaceBuildDataSkeleton,
} from "#/modules/workspaces/WorkspaceBuildData/WorkspaceBuildData";
import { WorkspaceBuildLogs } from "#/modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { cn } from "#/utils/cn";
import { formatDate } from "#/utils/time";
import { displayWorkspaceBuildDuration } from "#/utils/workspace";
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
	const [searchParams, setSearchParams] = useSearchParams();

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

	const agents = build.resources.flatMap((resource) => resource.agents ?? []);
	const logsParam = searchParams.get(LOGS_TAB_KEY);
	const selectedTab =
		logsParam && agents.some((agent) => agent.id === logsParam)
			? logsParam
			: "build";

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
						{formatDate(new Date(build.created_at))}
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
					<div className="flex items-center justify-between border-0 border-b border-solid border-border relative">
						<Tabs
							value={selectedTab}
							onValueChange={(value: string) => {
								setSearchParams((previous) => {
									const next = new URLSearchParams(previous);
									if (value === "build") {
										next.delete(LOGS_TAB_KEY);
									} else {
										next.set(LOGS_TAB_KEY, value);
									}
									return next;
								});
							}}
							className="w-full -m-px"
						>
							<TabsList variant="insideBox">
								<TabsTrigger value="build">Build</TabsTrigger>
								{agents.map((agent) => (
									<TabsTrigger value={agent.id} key={agent.id}>
										coder_agent.{agent.name}
									</TabsTrigger>
								))}
							</TabsList>
							<TabsContent value="build">
								<div className="p-2 flex justify-end absolute right-0 top-0">
									<Button asChild size="sm" variant="outline">
										<a
											href={`/api/v2/workspacebuilds/${build.id}/logs?format=text`}
											target="_blank"
											rel="noopener noreferrer"
										>
											View raw logs
											<ExternalLinkIcon className="size-3" />
										</a>
									</Button>
								</div>
								{build.transition === "delete" &&
									build.job.status === "failed" && (
										<Alert
											severity="error"
											prominent
											className="rounded-none border-0 border-b border-solid border-border"
										>
											<div>
												The workspace may have failed to delete due to a
												Terraform state mismatch. A template admin may run{" "}
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
										prominent
										className="rounded-none border-0 border-b border-solid border-border"
									>
										Provisioner logs exceeded the max size of 1MB. Will not
										continue to write provisioner logs for workspace build.
									</Alert>
								)}
								<BuildLogsContent logs={logs} build={build} />
							</TabsContent>
							{agents.map((agent) => (
								<TabsContent value={agent.id} key={agent.id}>
									<div className="p-2 flex justify-end absolute right-0 top-0">
										<Button asChild size="sm" variant="outline">
											<a
												href={`/api/v2/workspaceagents/${agent.id}/logs?format=text`}
												target="_blank"
												rel="noopener noreferrer"
											>
												View raw logs
												<ExternalLinkIcon className="size-3" />
											</a>
										</Button>
									</div>
									<AgentLogsContent agent={agent} />
								</TabsContent>
							))}
						</Tabs>
					</div>
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
			className="border-none"
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

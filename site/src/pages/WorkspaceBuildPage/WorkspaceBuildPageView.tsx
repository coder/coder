import type { Interpolation, Theme } from "@emotion/react";
import type {
	ProvisionerJobLog,
	WorkspaceAgent,
	WorkspaceBuild,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import type { Line } from "components/Logs/LogLine";
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
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import { displayWorkspaceBuildDuration } from "utils/workspace";
import { Sidebar, SidebarCaption, SidebarItem } from "./Sidebar";

export const LOGS_TAB_KEY = "logs";

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

				<Stats aria-label="Build details" css={styles.stats}>
					<StatsItem
						css={styles.statsItem}
						label="Workspace"
						value={
							<Link
								to={`/@${build.workspace_owner_name}/${build.workspace_name}`}
							>
								{build.workspace_name}
							</Link>
						}
					/>
					<StatsItem
						css={styles.statsItem}
						label="Template version"
						value={build.template_version_name}
					/>
					<StatsItem
						css={styles.statsItem}
						label="Duration"
						value={displayWorkspaceBuildDuration(build)}
					/>
					<StatsItem
						css={styles.statsItem}
						label="Started at"
						value={new Date(build.created_at).toLocaleString()}
					/>
					<StatsItem
						css={styles.statsItem}
						label="Action"
						value={<span className="uppercase">{build.transition}</span>}
					/>
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
						<TabsList>
							<TabLink to={`?${LOGS_TAB_KEY}=build`} value="build">
								Build
							</TabLink>

							{agents.map((a) => (
								<TabLink
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

const ScrollArea: FC<HTMLProps<HTMLDivElement>> = ({
	className,
	style = {},
	...delegatedProps
}) => {
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
	const [height, setHeight] = useState(0);
	useLayoutEffect(() => {
		const contentEl = contentRef.current;
		if (!contentEl) {
			return;
		}

		const syncParentSize = () => {
			const parentEl = contentEl.parentElement;
			if (parentEl) {
				setHeight(parentEl.clientHeight);
			}
		};

		syncParentSize();
		const resizeObserver = new ResizeObserver(syncParentSize);
		resizeObserver.observe(document.body);
		return () => resizeObserver.disconnect();
	}, []);

	return (
		<div
			ref={contentRef}
			className={cn("overflow-y-auto w-full", className)}
			style={{ ...style, height }}
			{...delegatedProps}
		/>
	);
};

const BuildLogsContent: FC<{
	logs?: ProvisionerJobLog[];
	build?: WorkspaceBuild;
}> = ({ logs, build }) => {
	if (!logs) {
		return <Loader />;
	}

	return (
		<WorkspaceBuildLogs
			css={{
				border: 0,
				"--log-line-side-padding": `${TAB_PADDING_X}px`,
				// Add extra spacing to the first log header to prevent it from being
				// too close to the tabs
				"& .logs-header:first-of-type": {
					paddingTop: 16,
				},
			}}
			build={build}
			logs={[...logs].sort((a, b) => {
				return (
					new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
				);
			})}
		/>
	);
};

type AgentLogsContentProps = {
	agent: WorkspaceAgent;
};

const AgentLogsContent: FC<AgentLogsContentProps> = ({ agent }) => {
	const logs = useAgentLogs(agent, true);

	if (!logs) {
		return <Loader />;
	}

	return (
		<AgentLogs
			sources={agent.log_sources}
			logs={logs.map<Line>((l) => ({
				id: l.id,
				output: l.output,
				time: l.created_at,
				level: l.level,
				sourceId: l.source_id,
			}))}
			height={560}
			width="100%"
		/>
	);
};

const styles = {
	stats: (theme) => ({
		padding: 0,
		border: 0,
		gap: 48,
		rowGap: 24,
		flex: 1,

		[theme.breakpoints.down("md")]: {
			display: "flex",
			flexDirection: "column",
			alignItems: "flex-start",
			gap: 8,
		},
	}),

	statsItem: {
		flexDirection: "column",
		gap: 0,
		padding: 0,

		"& > span:first-of-type": {
			fontSize: 12,
			fontWeight: 500,
		},
	},
} satisfies Record<string, Interpolation<Theme>>;

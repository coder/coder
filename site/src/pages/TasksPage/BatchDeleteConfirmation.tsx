import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { visuallyHidden } from "@mui/utils";
import type { Task, Workspace } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { ClockIcon, ServerIcon, UserIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { getResourceIconPath } from "utils/workspace";

dayjs.extend(relativeTime);

type BatchDeleteConfirmationProps = {
	checkedTasks: readonly Task[];
	workspaces: readonly Workspace[];
	open: boolean;
	isLoading: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

export const BatchDeleteConfirmation: FC<BatchDeleteConfirmationProps> = ({
	checkedTasks,
	workspaces,
	open,
	onClose,
	onConfirm,
	isLoading,
}) => {
	const [stage, setStage] = useState<"consequences" | "tasks" | "resources">(
		"consequences",
	);

	const onProceed = () => {
		switch (stage) {
			case "resources":
				onConfirm();
				break;
			case "tasks":
				setStage("resources");
				break;
			case "consequences":
				setStage("tasks");
				break;
		}
	};

	const taskCount = `${checkedTasks.length} ${
		checkedTasks.length === 1 ? "task" : "tasks"
	}`;

	let confirmText: ReactNode = <>Review selected tasks&hellip;</>;
	if (stage === "tasks") {
		confirmText = <>Confirm {taskCount}&hellip;</>;
	}
	if (stage === "resources") {
		const workspaceCount = workspaces.length;
		const resources = workspaces
			.map((workspace) => workspace.latest_build.resources.length)
			.reduce((a, b) => a + b, 0);
		const resourceCount = `${resources} ${
			resources === 1 ? "resource" : "resources"
		}`;
		const workspaceCountText = `${workspaceCount} ${
			workspaceCount === 1 ? "workspace" : "workspaces"
		}`;
		confirmText = (
			<>
				Delete {taskCount}, {workspaceCountText} and {resourceCount}
			</>
		);
	}

	// The flicker of these icons is quite noticeable if they aren't
	// loaded in advance, so we insert them into the document without
	// actually displaying them yet.
	const resourceIconPreloads = [
		...new Set(
			workspaces.flatMap((workspace) =>
				workspace.latest_build.resources.map(
					(resource) => resource.icon || getResourceIconPath(resource.type),
				),
			),
		),
	].map((url) => (
		<img key={url} alt="" aria-hidden css={{ ...visuallyHidden }} src={url} />
	));

	return (
		<ConfirmDialog
			type="delete"
			open={open}
			onClose={() => {
				setStage("consequences");
				onClose();
			}}
			title={`Delete ${taskCount}`}
			hideCancel
			confirmLoading={isLoading}
			confirmText={confirmText}
			onConfirm={onProceed}
			description={
				<>
					{stage === "consequences" && <Consequences />}
					{stage === "tasks" && <Tasks tasks={checkedTasks} />}
					{stage === "resources" && (
						<Resources tasks={checkedTasks} workspaces={workspaces} />
					)}
					{resourceIconPreloads}
				</>
			}
		/>
	);
};

interface TasksStageProps {
	tasks: readonly Task[];
}

interface ResourcesStageProps {
	tasks: readonly Task[];
	workspaces: readonly Workspace[];
}

const Consequences: FC = () => {
	return (
		<>
			<p>Deleting tasks is irreversible!</p>
			<ul css={styles.consequences}>
				<li>
					Tasks with associated workspaces will have those workspaces deleted.
				</li>
				<li>Terraform resources in task workspaces will be destroyed.</li>
				<li>Any data stored in task workspaces will be permanently deleted.</li>
			</ul>
		</>
	);
};

const Tasks: FC<TasksStageProps> = ({ tasks }) => {
	const theme = useTheme();

	const mostRecent = tasks.reduce(
		(latestSoFar, against) => {
			if (!latestSoFar) {
				return against;
			}

			return new Date(against.created_at).getTime() >
				new Date(latestSoFar.created_at).getTime()
				? against
				: latestSoFar;
		},
		undefined as Task | undefined,
	);

	const owners = new Set(tasks.map((it) => it.owner_name)).size;
	const ownersCount = `${owners} ${owners === 1 ? "owner" : "owners"}`;

	return (
		<>
			<ul css={styles.tasksList}>
				{tasks.map((task) => (
					<li key={task.id} css={styles.task}>
						<Stack
							direction="row"
							alignItems="center"
							justifyContent="space-between"
							spacing={3}
						>
							<span
								css={{
									fontWeight: 500,
									color: theme.experimental.l1.text,
									maxWidth: 400,
									overflow: "hidden",
									textOverflow: "ellipsis",
									whiteSpace: "nowrap",
								}}
							>
								{task.initial_prompt}
							</span>

							<Stack css={{ gap: 0, fontSize: 14 }} justifyContent="flex-end">
								<Stack
									direction="row"
									alignItems="center"
									justifyContent="flex-end"
									spacing={1}
								>
									<span css={{ whiteSpace: "nowrap" }}>{task.owner_name}</span>
									<PersonIcon />
								</Stack>
								<Stack
									direction="row"
									alignItems="center"
									spacing={1}
									justifyContent="flex-end"
								>
									<span css={{ whiteSpace: "nowrap" }}>
										{dayjs(task.created_at).fromNow()}
									</span>
									<ClockIcon className="size-icon-xs" />
								</Stack>
							</Stack>
						</Stack>
					</li>
				))}
			</ul>
			<Stack
				justifyContent="center"
				direction="row"
				wrap="wrap"
				css={{ gap: "6px 20px", fontSize: 14 }}
			>
				<Stack direction="row" alignItems="center" spacing={1}>
					<PersonIcon />
					<span>{ownersCount}</span>
				</Stack>
				{mostRecent && (
					<Stack direction="row" alignItems="center" spacing={1}>
						<ClockIcon className="size-icon-xs" />
						<span>Last created {dayjs(mostRecent.created_at).fromNow()}</span>
					</Stack>
				)}
			</Stack>
		</>
	);
};

const Resources: FC<ResourcesStageProps> = ({ tasks, workspaces }) => {
	const resources: Record<string, { count: number; icon: string }> = {};
	for (const workspace of workspaces) {
		for (const resource of workspace.latest_build.resources) {
			if (!resources[resource.type]) {
				resources[resource.type] = {
					count: 0,
					icon: resource.icon || getResourceIconPath(resource.type),
				};
			}

			resources[resource.type].count++;
		}
	}

	return (
		<Stack>
			<p>
				Deleting {tasks.length === 1 ? "this task" : "these tasks"} will also
				permanently destroy&hellip;
			</p>
			<Stack
				direction="row"
				justifyContent="center"
				wrap="wrap"
				css={{ gap: "6px 20px", fontSize: 14 }}
			>
				<Stack direction="row" alignItems="center" spacing={1}>
					<ServerIcon className="size-icon-sm" />
					<span>
						{workspaces.length}{" "}
						{workspaces.length === 1 ? "workspace" : "workspaces"}
					</span>
				</Stack>
				{Object.entries(resources).map(([type, summary]) => (
					<Stack key={type} direction="row" alignItems="center" spacing={1}>
						<ExternalImage
							src={summary.icon}
							width={styles.summaryIcon.width}
							height={styles.summaryIcon.height}
						/>
						<span>
							{summary.count} <code>{type}</code>
						</span>
					</Stack>
				))}
			</Stack>
		</Stack>
	);
};

const PersonIcon: FC = () => {
	return <UserIcon className="size-icon-sm" css={{ margin: -1 }} />;
};

const styles = {
	summaryIcon: { width: 16, height: 16 },

	consequences: {
		display: "flex",
		flexDirection: "column",
		gap: 8,
		paddingLeft: 16,
		marginBottom: 0,
	},

	tasksList: (theme) => ({
		listStyleType: "none",
		padding: 0,
		border: `1px solid ${theme.palette.divider}`,
		borderRadius: 8,
		overflow: "hidden auto",
		maxHeight: 184,
	}),

	task: (theme) => ({
		padding: "8px 16px",
		borderBottom: `1px solid ${theme.palette.divider}`,

		"&:last-child": {
			border: "none",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;

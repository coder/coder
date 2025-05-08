import { InstallDesktopIcon as InstallDesktopIcon, PersonOutlinedIcon as PersonOutlinedIcon, ScheduleIcon as ScheduleIcon, SettingsSuggestIcon as SettingsSuggestIcon } from "lucide-react";
import type { Interpolation, Theme } from "@emotion/react";
import { API } from "api/api";
import type { TemplateVersion, Workspace } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Loader } from "components/Loader/Loader";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { type FC, type ReactNode, useEffect, useMemo, useState } from "react";
import { useQueries } from "react-query";

dayjs.extend(relativeTime);

type BatchUpdateConfirmationProps = {
	checkedWorkspaces: readonly Workspace[];
	open: boolean;
	isLoading: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

export interface Update extends TemplateVersion {
	template_display_name: string;
	affected_workspaces: readonly Workspace[];
}

export const BatchUpdateConfirmation: FC<BatchUpdateConfirmationProps> = ({
	checkedWorkspaces,
	open,
	onClose,
	onConfirm,
	isLoading,
}) => {
	// Ignore workspaces with no pending update
	const outdatedWorkspaces = useMemo(
		() => checkedWorkspaces.filter((workspace) => workspace.outdated),
		[checkedWorkspaces],
	);

	// Separate out dormant workspaces. You cannot update a dormant workspace without
	// activate it, so notify the user that these selected workspaces will not be updated.
	const [dormantWorkspaces, workspacesToUpdate] = useMemo(() => {
		const dormantWorkspaces = [];
		const workspacesToUpdate = [];

		for (const it of outdatedWorkspaces) {
			if (it.dormant_at) {
				dormantWorkspaces.push(it);
			} else {
				workspacesToUpdate.push(it);
			}
		}

		return [dormantWorkspaces, workspacesToUpdate];
	}, [outdatedWorkspaces]);

	// We need to know which workspaces are running, so we can provide more detailed
	// warnings about them
	const runningWorkspacesToUpdate = useMemo(
		() =>
			workspacesToUpdate.filter(
				(workspace) => workspace.latest_build.status === "running",
			),
		[workspacesToUpdate],
	);

	// If there aren't any running _and_ outdated workspaces selected, we can skip
	// the consequences page, since an update shouldn't have any consequences that
	// the stop didn't already. If there are dormant workspaces but no running
	// workspaces, start there instead.
	const [stage, setStage] = useState<
		"consequences" | "dormantWorkspaces" | "updates" | null
	>(null);
	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useEffect(() => {
		if (runningWorkspacesToUpdate.length > 0) {
			setStage("consequences");
		} else if (dormantWorkspaces.length > 0) {
			setStage("dormantWorkspaces");
		} else {
			setStage("updates");
		}
	}, [runningWorkspacesToUpdate, dormantWorkspaces, checkedWorkspaces, open]);

	// Figure out which new versions everything will be updated to so that we can
	// show update messages and such.
	const newVersions = useMemo(() => {
		type MutableUpdateInfo = {
			id: string;
			template_display_name: string;
			affected_workspaces: Workspace[];
		};

		const newVersions = new Map<string, MutableUpdateInfo>();
		for (const it of workspacesToUpdate) {
			const versionId = it.template_active_version_id;
			const version = newVersions.get(versionId);

			if (version) {
				version.affected_workspaces.push(it);
				continue;
			}

			newVersions.set(versionId, {
				id: versionId,
				template_display_name: it.template_display_name,
				affected_workspaces: [it],
			});
		}

		type ReadonlyUpdateInfo = Readonly<MutableUpdateInfo> & {
			affected_workspaces: readonly Workspace[];
		};

		return newVersions as Map<string, ReadonlyUpdateInfo>;
	}, [workspacesToUpdate]);

	// Not all of the information we want is included in the `Workspace` type, so we
	// need to query all of the versions.
	const results = useQueries({
		queries: [...newVersions.values()].map((version) => ({
			queryKey: ["batchUpdate", version.id],
			queryFn: async () => ({
				// ...but the query _also_ doesn't have everything we need, like the
				// template display name!
				...version,
				...(await API.getTemplateVersion(version.id)),
			}),
		})),
	});
	const { data, error } = {
		data: results.every((result) => result.isSuccess && result.data)
			? results.map((result) => result.data!)
			: undefined,
		error: results.some((result) => result.error),
	};

	const onProceed = () => {
		switch (stage) {
			case "updates":
				onConfirm();
				break;
			case "dormantWorkspaces":
				setStage("updates");
				break;
			case "consequences":
				setStage(
					dormantWorkspaces.length > 0 ? "dormantWorkspaces" : "updates",
				);
				break;
		}
	};

	const workspaceCount = `${workspacesToUpdate.length} ${
		workspacesToUpdate.length === 1 ? "workspace" : "workspaces"
	}`;

	let confirmText: ReactNode = <>Review updates&hellip;</>;
	if (stage === "updates") {
		confirmText = <>Update {workspaceCount}</>;
	}

	return (
		<ConfirmDialog
			open={open}
			onClose={onClose}
			title={`Update ${workspaceCount}`}
			hideCancel
			confirmLoading={isLoading}
			confirmText={confirmText}
			onConfirm={onProceed}
			description={
				<>
					{stage === "consequences" && (
						<Consequences runningWorkspaces={runningWorkspacesToUpdate} />
					)}
					{stage === "dormantWorkspaces" && (
						<DormantWorkspaces workspaces={dormantWorkspaces} />
					)}
					{stage === "updates" && (
						<Updates
							workspaces={workspacesToUpdate}
							updates={data}
							error={error}
						/>
					)}
				</>
			}
		/>
	);
};

interface ConsequencesProps {
	runningWorkspaces: Workspace[];
}

const Consequences: FC<ConsequencesProps> = ({ runningWorkspaces }) => {
	const workspaceCount = `${runningWorkspaces.length} ${
		runningWorkspaces.length === 1 ? "running workspace" : "running workspaces"
	}`;

	const owners = new Set(runningWorkspaces.map((it) => it.owner_id)).size;
	const ownerCount = `${owners} ${owners === 1 ? "owner" : "owners"}`;

	return (
		<>
			<p>You are about to update {workspaceCount}.</p>
			<ul css={styles.consequences}>
				<li>
					Updating will start workspaces on their latest template versions. This
					can delete non-persistent data.
				</li>
				<li>
					Anyone connected to a running workspace will be disconnected until the
					update is complete.
				</li>
				<li>Any unsaved data will be lost.</li>
			</ul>
			<Stack
				justifyContent="center"
				direction="row"
				wrap="wrap"
				css={styles.summary}
			>
				<Stack direction="row" alignItems="center" spacing={1}>
					<PersonIcon />
					<span>{ownerCount}</span>
				</Stack>
			</Stack>
		</>
	);
};

interface DormantWorkspacesProps {
	workspaces: Workspace[];
}

const DormantWorkspaces: FC<DormantWorkspacesProps> = ({ workspaces }) => {
	const mostRecent = workspaces.reduce(
		(latestSoFar, against) => {
			if (!latestSoFar) {
				return against;
			}

			return new Date(against.last_used_at).getTime() >
				new Date(latestSoFar.last_used_at).getTime()
				? against
				: latestSoFar;
		},
		undefined as Workspace | undefined,
	);

	const owners = new Set(workspaces.map((it) => it.owner_id)).size;
	const ownersCount = `${owners} ${owners === 1 ? "owner" : "owners"}`;

	return (
		<>
			<p>
				{workspaces.length === 1 ? (
					<>
						This selected workspace is dormant, and must be activated before it
						can be updated.
					</>
				) : (
					<>
						These selected workspaces are dormant, and must be activated before
						they can be updated.
					</>
				)}
			</p>
			<ul css={styles.workspacesList}>
				{workspaces.map((workspace) => (
					<li key={workspace.id} css={styles.workspace}>
						<Stack
							direction="row"
							alignItems="center"
							justifyContent="space-between"
						>
							<span css={styles.name}>{workspace.name}</span>
							<Stack css={{ gap: 0, fontSize: 14, width: 128 }}>
								<Stack direction="row" alignItems="center" spacing={1}>
									<PersonIcon />
									<span
										css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}
									>
										{workspace.owner_name}
									</span>
								</Stack>
								<Stack direction="row" alignItems="center" spacing={1}>
									<ScheduleIcon css={styles.summaryIcon} />
									<span
										css={{ whiteSpace: "nowrap", textOverflow: "ellipsis" }}
									>
										{lastUsed(workspace.last_used_at)}
									</span>
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
				css={styles.summary}
			>
				<Stack direction="row" alignItems="center" spacing={1}>
					<PersonIcon />
					<span>{ownersCount}</span>
				</Stack>
				{mostRecent && (
					<Stack direction="row" alignItems="center" spacing={1}>
						<ScheduleIcon css={styles.summaryIcon} />
						<span>Last used {lastUsed(mostRecent.last_used_at)}</span>
					</Stack>
				)}
			</Stack>
		</>
	);
};

interface UpdatesProps {
	workspaces: Workspace[];
	updates?: Update[];
	error?: unknown;
}

const Updates: FC<UpdatesProps> = ({ workspaces, updates, error }) => {
	const workspaceCount = `${workspaces.length} ${
		workspaces.length === 1 ? "outdated workspace" : "outdated workspaces"
	}`;

	const updateCount =
		updates &&
		`${updates.length} ${
			updates.length === 1 ? "new version" : "new versions"
		}`;

	return (
		<>
			<TemplateVersionMessages updates={updates} error={error} />
			<Stack
				justifyContent="center"
				direction="row"
				wrap="wrap"
				css={styles.summary}
			>
				<Stack direction="row" alignItems="center" spacing={1}>
					<InstallDesktopIcon css={styles.summaryIcon} />
					<span>{workspaceCount}</span>
				</Stack>
				{updateCount && (
					<Stack direction="row" alignItems="center" spacing={1}>
						<SettingsSuggestIcon css={styles.summaryIcon} />
						<span>{updateCount}</span>
					</Stack>
				)}
			</Stack>
		</>
	);
};

interface TemplateVersionMessagesProps {
	error?: unknown;
	updates?: Update[];
}

const TemplateVersionMessages: FC<TemplateVersionMessagesProps> = ({
	error,
	updates,
}) => {
	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (!updates) {
		return <Loader />;
	}

	return (
		<ul css={styles.updatesList}>
			{updates.map((update) => (
				<li key={update.id} css={styles.workspace}>
					<Stack spacing={0}>
						<Stack spacing={0.5} direction="row" alignItems="center">
							<span css={styles.name}>{update.template_display_name}</span>
							<span css={styles.newVersion}>&rarr; {update.name}</span>
						</Stack>
						<MemoizedInlineMarkdown
							allowedElements={["ol", "ul", "li"]}
							css={styles.message}
						>
							{update.message ?? "No message"}
						</MemoizedInlineMarkdown>
						<UsedBy workspaces={update.affected_workspaces} />
					</Stack>
				</li>
			))}
		</ul>
	);
};

interface UsedByProps {
	workspaces: readonly Workspace[];
}

const UsedBy: FC<UsedByProps> = ({ workspaces }) => {
	const workspaceNames = workspaces.map((it) => it.name);

	return (
		<p css={{ fontSize: 13, paddingTop: 6, lineHeight: 1.2 }}>
			Used by {workspaceNames.slice(0, 2).join(", ")}{" "}
			{workspaceNames.length > 2 && (
				<span title={workspaceNames.slice(2).join(", ")}>
					and {workspaceNames.length - 2} more
				</span>
			)}
		</p>
	);
};

const lastUsed = (time: string) => {
	const now = dayjs();
	const then = dayjs(time);
	return then.isAfter(now.subtract(1, "hour")) ? "now" : then.fromNow();
};

const PersonIcon: FC = () => {
	// This size doesn't match the rest of the icons because MUI is just really
	// inconsistent. We have to make it bigger than the rest, and pull things in
	// on the sides to compensate.
	return <PersonOutlinedIcon css={{ width: 18, height: 18, margin: -1 }} />;
};

const styles = {
	summaryIcon: { width: 16, height: 16 },

	consequences: {
		display: "flex",
		flexDirection: "column",
		gap: 8,
		paddingLeft: 16,
	},

	workspacesList: (theme) => ({
		listStyleType: "none",
		padding: 0,
		border: `1px solid ${theme.palette.divider}`,
		borderRadius: 8,
		overflow: "hidden auto",
		maxHeight: 184,
	}),

	updatesList: (theme) => ({
		listStyleType: "none",
		padding: 0,
		border: `1px solid ${theme.palette.divider}`,
		borderRadius: 8,
		overflow: "hidden auto",
		maxHeight: 256,
	}),

	workspace: (theme) => ({
		padding: "8px 16px",
		borderBottom: `1px solid ${theme.palette.divider}`,

		"&:last-child": {
			border: "none",
		},
	}),

	name: (theme) => ({
		fontWeight: 500,
		color: theme.experimental.l1.text,
	}),

	newVersion: (theme) => ({
		fontSize: 13,
		fontWeight: 500,
		color: theme.roles.active.fill.solid,
	}),

	message: {
		fontSize: 14,
	},

	summary: {
		gap: "6px 20px",
		fontSize: 14,
	},
} satisfies Record<string, Interpolation<Theme>>;

import type { Workspace } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { ClockIcon, UserIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { getResourceIconPath } from "utils/workspace";

dayjs.extend(relativeTime);

type BatchDeleteConfirmationProps = {
	checkedWorkspaces: readonly Workspace[];
	open: boolean;
	isLoading: boolean;
	onClose: () => void;
	onConfirm: () => void;
};

export const BatchDeleteConfirmation: FC<BatchDeleteConfirmationProps> = ({
	checkedWorkspaces,
	open,
	onClose,
	onConfirm,
	isLoading,
}) => {
	const [stage, setStage] = useState<
		"consequences" | "workspaces" | "resources"
	>("consequences");

	const onProceed = () => {
		switch (stage) {
			case "resources":
				onConfirm();
				break;
			case "workspaces":
				setStage("resources");
				break;
			case "consequences":
				setStage("workspaces");
				break;
		}
	};

	const workspaceCount = `${checkedWorkspaces.length} ${
		checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
	}`;

	let confirmText: ReactNode = <>Review selected workspaces&hellip;</>;
	if (stage === "workspaces") {
		confirmText = <>Confirm {workspaceCount}&hellip;</>;
	}
	if (stage === "resources") {
		const resources = checkedWorkspaces
			.map((workspace) => workspace.latest_build.resources.length)
			.reduce((a, b) => a + b, 0);
		const resourceCount = `${resources} ${
			resources === 1 ? "resource" : "resources"
		}`;
		confirmText = (
			<>
				Delete {workspaceCount} and {resourceCount}
			</>
		);
	}

	// The flicker of these icons is quite noticeable if they aren't loaded in advance,
	// so we insert them into the document without actually displaying them yet.
	const resourceIconPreloads = [
		...new Set(
			checkedWorkspaces.flatMap((workspace) =>
				workspace.latest_build.resources.map(
					(resource) => resource.icon || getResourceIconPath(resource.type),
				),
			),
		),
	].map((url) => (
		<img key={url} alt="" aria-hidden className="sr-only" src={url} />
	));

	return (
		<ConfirmDialog
			type="delete"
			open={open}
			onClose={() => {
				setStage("consequences");
				onClose();
			}}
			title={`Delete ${workspaceCount}`}
			hideCancel
			confirmLoading={isLoading}
			confirmText={confirmText}
			onConfirm={onProceed}
			description={
				<>
					{stage === "consequences" && <Consequences />}
					{stage === "workspaces" && (
						<Workspaces workspaces={checkedWorkspaces} />
					)}
					{stage === "resources" && (
						<Resources workspaces={checkedWorkspaces} />
					)}
					{resourceIconPreloads}
				</>
			}
		/>
	);
};

interface StageProps {
	workspaces: readonly Workspace[];
}

const Consequences: FC = () => {
	return (
		<>
			<p>Deleting workspaces is irreversible!</p>
			<ul className="flex flex-col gap-2 pl-4 mb-0">
				<li>
					Terraform resources belonging to deleted workspaces will be destroyed.
				</li>
				<li>Any data stored in the workspace will be permanently deleted.</li>
			</ul>
		</>
	);
};

const Workspaces: FC<StageProps> = ({ workspaces }) => {
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
			<ul className="list-none p-0 border border-solid border-zinc-200 dark:border-zinc-700 rounded-lg overflow-x-hidden overflow-y-auto max-h-[184px]">
				{workspaces.map((workspace) => (
					<li
						key={workspace.id}
						className="py-2 px-4 border-solid border-0 border-b border-zinc-200 dark:border-zinc-700 last:border-b-0"
					>
						<div className="flex items-center justify-between gap-6">
							<span className="font-medium text-content-primary max-w-[400px] overflow-hidden text-ellipsis whitespace-nowrap">
								{workspace.name}
							</span>

							<div className="flex flex-col text-sm items-end">
								<div className="flex items-center gap-2">
									<span className="whitespace-nowrap">
										{workspace.owner_name}
									</span>
									<UserIcon className="size-icon-sm -m-px" />
								</div>
								<div className="flex items-center gap-2">
									<span className="whitespace-nowrap">
										{dayjs(workspace.last_used_at).fromNow()}
									</span>
									<ClockIcon className="size-icon-xs" />
								</div>
							</div>
						</div>
					</li>
				))}
			</ul>
			<div className="flex flex-wrap justify-center gap-x-5 gap-y-1.5 text-sm">
				<div className="flex items-center gap-2">
					<UserIcon className="size-icon-sm -m-px" />
					<span>{ownersCount}</span>
				</div>
				{mostRecent && (
					<div className="flex items-center gap-2">
						<ClockIcon className="size-icon-xs" />
						<span>Last used {dayjs(mostRecent.last_used_at).fromNow()}</span>
					</div>
				)}
			</div>
		</>
	);
};

const Resources: FC<StageProps> = ({ workspaces }) => {
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
		<div className="flex flex-col gap-4">
			<p>
				Deleting{" "}
				{workspaces.length === 1 ? "this workspace" : "these workspaces"} will
				also permanently destroy&hellip;
			</p>
			<div className="flex flex-wrap justify-center gap-x-5 gap-y-1.5 text-sm">
				{Object.entries(resources).map(([type, summary]) => (
					<div key={type} className="flex items-center gap-2">
						<ExternalImage src={summary.icon} width={16} height={16} />
						<span>
							{summary.count} <code>{type}</code>
						</span>
					</div>
				))}
			</div>
		</div>
	);
};

import { UsersIcon } from "lucide-react";
import type { ChatProject } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Input } from "#/components/Input/Input";
import { Separator } from "#/components/Separator/Separator";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";
import { ChatProjectIcon } from "../ChatProjectIcon";
import { validateChatProjectName } from "../chatProjectFields";
import { AgentUserLink } from "./AgentUserLink";
import { InlineEditField } from "./InlineEditField";

type ProjectPageHeaderProps = {
	readonly project: ChatProject;
	/** The owner's display name. `undefined` while it loads. */
	readonly ownerLabel: string | undefined;
	/** Omit to hide the organization item. */
	readonly organizationLabel?: string;
	readonly isOwnedByOtherUser: boolean;
	/** Saves the trimmed name. Omit when the viewer cannot edit the agent. */
	readonly onSaveName?: (name: string) => Promise<unknown>;
	/** Who the agent is shared with, and the control to change it. */
	readonly sharing?: React.ReactNode;
	readonly actions?: React.ReactNode;
};

/**
 * The agent's name, edited in place, with a row of small facts about it:
 * its user, owner, organization, and who it is shared with.
 */
export const ProjectPageHeader: React.FC<ProjectPageHeaderProps> = ({
	project,
	ownerLabel,
	organizationLabel,
	isOwnedByOtherUser,
	onSaveName,
	sharing,
	actions,
}) => (
	<header className="flex min-w-0 flex-col gap-3">
		<div className="flex min-w-0 items-start gap-3">
			<div className="min-w-0 flex-1">
				<InlineEditField
					label="Name"
					value={project.name}
					fitContent
					validate={validateChatProjectName}
					onSave={onSaveName && ((value) => onSaveName(value.trim()))}
					renderValue={(value) => (
						<h1 className="m-0 truncate text-3xl font-semibold text-content-primary">
							{value}
						</h1>
					)}
					renderEditor={({
						value,
						onChange,
						id,
						invalid,
						describedBy,
						readOnly,
					}) => (
						<Input
							id={id}
							autoFocus
							aria-label="Name"
							aria-invalid={invalid}
							aria-describedby={describedBy}
							readOnly={readOnly}
							value={value}
							className="h-12 text-2xl font-semibold"
							onChange={(event) => onChange(event.target.value)}
						/>
					)}
				/>
			</div>
			{actions}
		</div>
		{project.description && (
			<p className="m-0 text-sm text-content-secondary [overflow-wrap:anywhere]">
				{project.description}
			</p>
		)}
		<div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-content-secondary">
			<AgentUserItem project={project} />
			<span>
				By{" "}
				{ownerLabel ?? (
					<>
						<Skeleton className="inline-block h-3 w-16 align-middle" />
						<span className="sr-only">Loading</span>
					</>
				)}
			</span>
			<MetaSeparator />
			<time dateTime={project.created_at}>
				Created {formatDateTime(project.created_at, DATE_FORMAT.MEDIUM_DATE)}
			</time>
			{organizationLabel !== undefined && (
				<>
					<MetaSeparator />
					<span>{organizationLabel}</span>
				</>
			)}
			{isOwnedByOtherUser && (
				<>
					<MetaSeparator />
					<Badge size="sm" variant="outline">
						<UsersIcon aria-hidden />
						Shared with you
					</Badge>
				</>
			)}
			{sharing && (
				<>
					<MetaSeparator />
					{sharing}
				</>
			)}
		</div>
	</header>
);

const MetaSeparator: React.FC = () => (
	<Separator orientation="vertical" className="h-4" />
);

type AgentUserItemProps = {
	readonly project: ChatProject;
};

/** The agent's icon, which is also its user's avatar, and the user's name. */
const AgentUserItem: React.FC<AgentUserItemProps> = ({ project }) => {
	if (!project.agent_user) {
		return null;
	}
	return (
		<>
			<AgentUserLink
				username={project.agent_user.username}
				className="flex items-center gap-2 text-content-secondary no-underline hover:text-content-primary"
			>
				<span className="flex size-5 shrink-0 items-center justify-center rounded border border-solid border-border bg-surface-secondary">
					<ChatProjectIcon project={project} className="size-3.5" />
				</span>
				@{project.agent_user.username}
			</AgentUserLink>
			<MetaSeparator />
		</>
	);
};

import { useMutation, useQueryClient } from "react-query";
import { updateChatProject } from "#/api/queries/chatProjects";
import type {
	ChatProject,
	UpdateChatProjectRequest,
} from "#/api/typesGenerated";
import { IconField } from "#/components/IconField/IconField";
import { ChatProjectIcon } from "../ChatProjectIcon";
import {
	validateChatProjectDescription,
	validateChatProjectIcon,
} from "../chatProjectFields";
import { InlineEditField } from "./InlineEditField";
import { SettingsCard, SettingsCardRow, SettingsGroup } from "./ProjectSection";

type ProjectGeneralSectionProps = {
	readonly project: ChatProject;
};

/** Saves the agent's description and icon in place. */
export const ProjectGeneralSection: React.FC<ProjectGeneralSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const updateMutation = useMutation(updateChatProject(queryClient));
	const canEdit = project.permissions.edit;
	const save = (request: UpdateChatProjectRequest) =>
		updateMutation.mutateAsync({ project, request });

	return (
		<ProjectGeneralSectionView
			description={project.description}
			icon={project.icon}
			agentUsername={project.agent_user?.username}
			onSaveDescription={
				canEdit ? (description) => save({ description }) : undefined
			}
			onSaveIcon={canEdit ? (icon) => save({ icon }) : undefined}
		/>
	);
};

type ProjectGeneralSectionViewProps = {
	readonly description: string;
	readonly icon: string;
	/** Named in the section's description. Omit for agents without a user. */
	readonly agentUsername?: string;
	/** Saves the trimmed description. Omit when the viewer cannot edit. */
	readonly onSaveDescription?: (description: string) => Promise<unknown>;
	/** Saves the trimmed icon URL. Omit when the viewer cannot edit. */
	readonly onSaveIcon?: (icon: string) => Promise<unknown>;
};

/** The agent's description and icon, edited in place. */
export const ProjectGeneralSectionView: React.FC<
	ProjectGeneralSectionViewProps
> = ({ description, icon, agentUsername, onSaveDescription, onSaveIcon }) => (
	<SettingsGroup
		title="General"
		description={
			agentUsername
				? `Chats and workspaces in this agent belong to @${agentUsername}.`
				: undefined
		}
	>
		<SettingsCard as="div">
			<SettingsCardRow as="div" className="items-start">
				<span className="w-24 shrink-0 text-content-secondary">
					Description
				</span>
				<div className="min-w-0 flex-1">
					<InlineEditField
						label="Description"
						value={description}
						multiline
						validate={validateChatProjectDescription}
						onSave={
							onSaveDescription && ((value) => onSaveDescription(value.trim()))
						}
					/>
				</div>
			</SettingsCardRow>
			<SettingsCardRow as="div" className="items-start">
				<span className="w-24 shrink-0 text-content-secondary">Icon</span>
				<div className="min-w-0 flex-1">
					<InlineEditField
						label="Icon"
						value={icon}
						emptyLabel="Default"
						saveOnBlur={false}
						validate={validateChatProjectIcon}
						onSave={onSaveIcon && ((value) => onSaveIcon(value.trim()))}
						renderValue={(value) => (
							<ChatProjectIcon project={{ icon: value }} className="size-5" />
						)}
						renderEditor={({
							onChange,
							value,
							id,
							invalid,
							describedBy,
							readOnly,
						}) => (
							<IconField
								id={id}
								label={null}
								aria-label="Icon"
								value={value}
								readOnly={readOnly}
								error={invalid}
								aria-describedby={describedBy}
								autoFocus
								onChange={(event) => onChange(event.target.value)}
								onPickEmoji={onChange}
							/>
						)}
					/>
				</div>
			</SettingsCardRow>
		</SettingsCard>
	</SettingsGroup>
);

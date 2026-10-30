import { SlidersHorizontalIcon } from "lucide-react";
import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { updateChatProject } from "#/api/queries/chatProjects";
import { externalAuths } from "#/api/queries/externalAuth";
import { templates } from "#/api/queries/templates";
import type {
	ChatProject,
	ChatProjectTemplateParameterRule,
	UpdateChatProjectRequest,
} from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Label } from "#/components/Label/Label";
import { Switch } from "#/components/Switch/Switch";
import { AgentParameterRulesDialog } from "./AgentParameterRulesDialog";
import { OptionCheckbox } from "./OptionCheckbox";
import {
	SectionError,
	SectionLoadState,
	SectionSaveError,
	SettingsCard,
	SettingsCardEmptyRow,
	SettingsCardRow,
	SettingsSection,
} from "./ProjectSection";
import { useSingleFlight } from "./useSingleFlight";

type SelectableOption = {
	readonly id: string;
	readonly label: string;
	readonly isSelected: boolean;
};

type TemplateRow = SelectableOption & {
	/** True when this is the only allowed template, so it cannot be unchecked. */
	readonly isOnlyAllowed: boolean;
	readonly restrictedParameterCount: number;
};

const toggleId = (ids: readonly string[], id: string, checked: boolean) =>
	checked ? [...ids, id] : ids.filter((current) => current !== id);

type AgentWorkspacesSectionProps = {
	readonly project: ChatProject;
};

/**
 * The workspace tools switch, the allowed templates, and their external auth
 * providers. Changes save as they are made.
 */
export const AgentWorkspacesSection: React.FC<AgentWorkspacesSectionProps> = ({
	project,
}) => {
	const queryClient = useQueryClient();
	const templatesQuery = useQuery(templates());
	const externalAuthQuery = useQuery(externalAuths());
	const updateMutation = useMutation(updateChatProject(queryClient));
	const [configuringTemplateId, setConfiguringTemplateId] = useState<string>();
	const singleFlight = useSingleFlight();

	const orgTemplates = (templatesQuery.data ?? []).filter(
		(t) => t.organization_id === project.organization_id,
	);
	// An empty list allows every template.
	const allowedTemplateIds =
		project.allowed_template_ids.length === 0
			? orgTemplates.map((t) => t.id)
			: project.allowed_template_ids;
	const templateRows: TemplateRow[] | undefined = templatesQuery.data
		? orgTemplates.map((t) => ({
				id: t.id,
				label: t.display_name || t.name,
				isSelected: allowedTemplateIds.includes(t.id),
				isOnlyAllowed:
					allowedTemplateIds.length === 1 && allowedTemplateIds[0] === t.id,
				restrictedParameterCount: project.template_parameter_rules.filter(
					(r) => r.template_id === t.id,
				).length,
			}))
		: undefined;
	const externalAuthOptions: SelectableOption[] | undefined =
		externalAuthQuery.data?.providers.map((p) => ({
			id: p.id,
			label: p.display_name || p.id,
			isSelected: project.external_auth_provider_ids.includes(p.id),
		}));

	const save = (request: UpdateChatProjectRequest, onSuccess?: () => void) => {
		singleFlight.run((onSettled) =>
			updateMutation.mutate({ project, request }, { onSuccess, onSettled }),
		);
	};

	const toggleTemplate = (templateId: string, checked: boolean) => {
		const next = toggleId(allowedTemplateIds, templateId, checked);
		const allowsAll = orgTemplates.every((t) => next.includes(t.id));
		// A template that is no longer allowed loses its parameter restrictions.
		save({
			allowed_template_ids: allowsAll ? [] : next,
			template_parameter_rules: project.template_parameter_rules.filter(
				(r) => allowsAll || next.includes(r.template_id),
			),
		});
	};

	const configuringTemplate = orgTemplates.find(
		(t) => t.id === configuringTemplateId,
	);

	return (
		<>
			<AgentWorkspacesSectionView
				templates={templateRows}
				templatesError={templatesQuery.error}
				onRetryTemplates={() => void templatesQuery.refetch()}
				externalAuthProviders={externalAuthOptions}
				externalAuthError={externalAuthQuery.error}
				onRetryExternalAuth={() => void externalAuthQuery.refetch()}
				canEdit={project.permissions.edit}
				isSaving={updateMutation.isPending}
				// The dialog shows its own error while it is open.
				saveError={configuringTemplate ? undefined : updateMutation.error}
				workspaceToolsEnabled={project.workspace_tools_enabled}
				onWorkspaceToolsChange={(enabled) =>
					save({ workspace_tools_enabled: enabled })
				}
				onToggleTemplate={toggleTemplate}
				onConfigureParameters={(templateId) => {
					// Drop the error of an earlier failed change.
					updateMutation.reset();
					setConfiguringTemplateId(templateId);
				}}
				onToggleExternalAuthProvider={(id, checked) =>
					save({
						external_auth_provider_ids: toggleId(
							project.external_auth_provider_ids,
							id,
							checked,
						),
					})
				}
			/>
			{configuringTemplate && (
				<AgentParameterRulesDialog
					template={{
						id: configuringTemplate.id,
						label: configuringTemplate.display_name || configuringTemplate.name,
						activeVersionId: configuringTemplate.active_version_id,
					}}
					rules={project.template_parameter_rules.filter(
						(r) => r.template_id === configuringTemplate.id,
					)}
					isSaving={updateMutation.isPending}
					error={updateMutation.error}
					onClose={() => {
						updateMutation.reset();
						setConfiguringTemplateId(undefined);
					}}
					onSave={(rules: ChatProjectTemplateParameterRule[]) =>
						save(
							{
								template_parameter_rules: [
									...project.template_parameter_rules.filter(
										(r) => r.template_id !== configuringTemplate.id,
									),
									...rules,
								],
							},
							() => setConfiguringTemplateId(undefined),
						)
					}
				/>
			)}
		</>
	);
};

type AgentWorkspacesSectionViewProps = {
	/** `undefined` until the templates load. */
	readonly templates: readonly TemplateRow[] | undefined;
	/** Shown with Retry, in place of the templates or above them. */
	readonly templatesError: unknown;
	readonly onRetryTemplates: () => void;
	/** `undefined` until the providers load. */
	readonly externalAuthProviders: readonly SelectableOption[] | undefined;
	/** Shown with Retry, in place of the providers or above them. */
	readonly externalAuthError: unknown;
	readonly onRetryExternalAuth: () => void;
	readonly canEdit: boolean;
	/**
	 * True while a change saves. Controls stay focusable but ignore input
	 * until it settles.
	 */
	readonly isSaving: boolean;
	/** Why the last change failed to save, shown under the section header. */
	readonly saveError: unknown;
	/** When off, the template and provider editors are hidden. */
	readonly workspaceToolsEnabled: boolean;
	readonly onWorkspaceToolsChange: (enabled: boolean) => void;
	readonly onToggleTemplate: (id: string, checked: boolean) => void;
	readonly onConfigureParameters: (templateId: string) => void;
	readonly onToggleExternalAuthProvider: (id: string, checked: boolean) => void;
};

/** The Workspaces section and, while workspace tools are on, its providers. */
export const AgentWorkspacesSectionView: React.FC<
	AgentWorkspacesSectionViewProps
> = ({
	templates,
	templatesError,
	onRetryTemplates,
	externalAuthProviders,
	externalAuthError,
	onRetryExternalAuth,
	canEdit,
	isSaving,
	saveError,
	workspaceToolsEnabled,
	onWorkspaceToolsChange,
	onToggleTemplate,
	onConfigureParameters,
	onToggleExternalAuthProvider,
}) => {
	const switchId = useId();
	return (
		<>
			<SettingsSection
				title="Workspaces"
				description={
					canEdit && workspaceToolsEnabled
						? "Chats can only create workspaces from the checked templates, and at least one must stay checked."
						: "Chats can only create workspaces from these templates."
				}
				actions={
					canEdit ? (
						<div className="flex items-center gap-2 text-sm text-content-primary">
							<Label htmlFor={switchId} className="font-normal">
								Workspace tools
							</Label>
							<Switch
								id={switchId}
								checked={workspaceToolsEnabled}
								aria-disabled={isSaving}
								className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
								onCheckedChange={(enabled) => {
									if (!isSaving) {
										onWorkspaceToolsChange(enabled);
									}
								}}
							/>
						</div>
					) : (
						<Badge size="sm" variant="outline">
							{workspaceToolsEnabled ? "On" : "Off"}
						</Badge>
					)
				}
			>
				{Boolean(saveError) && <SectionSaveError error={saveError} />}
				{workspaceToolsEnabled ? (
					<TemplatesCard
						templates={templates}
						error={templatesError}
						onRetry={onRetryTemplates}
						canEdit={canEdit}
						isSaving={isSaving}
						onToggle={onToggleTemplate}
						onConfigureParameters={onConfigureParameters}
					/>
				) : (
					<SettingsCard>
						<SettingsCardEmptyRow>
							Workspace tools are off. Chats in this agent can't create or use
							workspaces.
						</SettingsCardEmptyRow>
					</SettingsCard>
				)}
			</SettingsSection>
			{workspaceToolsEnabled && (
				<SettingsSection
					title="External auth providers"
					description="Templates that require an unchecked provider are hidden. All providers are allowed when none are checked."
				>
					{externalAuthProviders === undefined ? (
						<SectionLoadState
							error={externalAuthError}
							onRetry={onRetryExternalAuth}
						/>
					) : (
						<>
							{Boolean(externalAuthError) && (
								<SectionError
									error={externalAuthError}
									onRetry={onRetryExternalAuth}
								/>
							)}
							<SettingsCard className="max-h-80 overflow-y-auto">
								{externalAuthProviders.length === 0 && (
									<SettingsCardEmptyRow>
										No external auth providers are configured.
									</SettingsCardEmptyRow>
								)}
								{externalAuthProviders.map((provider) => (
									<SettingsCardRow key={provider.id}>
										<OptionCheckbox
											label={provider.label}
											checked={provider.isSelected}
											isSaving={isSaving}
											readOnly={!canEdit}
											onCheckedChange={(checked) =>
												onToggleExternalAuthProvider(provider.id, checked)
											}
										/>
									</SettingsCardRow>
								))}
							</SettingsCard>
						</>
					)}
				</SettingsSection>
			)}
		</>
	);
};

type TemplatesCardProps = {
	readonly templates: readonly TemplateRow[] | undefined;
	readonly error: unknown;
	readonly onRetry: () => void;
	readonly canEdit: boolean;
	readonly isSaving: boolean;
	readonly onToggle: (id: string, checked: boolean) => void;
	readonly onConfigureParameters: (templateId: string) => void;
};

/** The template rows, or why they are not there yet. */
const TemplatesCard: React.FC<TemplatesCardProps> = ({
	templates,
	error,
	onRetry,
	canEdit,
	isSaving,
	onToggle,
	onConfigureParameters,
}) => {
	if (templates === undefined) {
		return <SectionLoadState error={error} onRetry={onRetry} />;
	}
	return (
		<>
			{Boolean(error) && <SectionError error={error} onRetry={onRetry} />}
			<SettingsCard className="max-h-80 overflow-y-auto">
				{templates.length === 0 && (
					<SettingsCardEmptyRow>
						No templates are configured.
					</SettingsCardEmptyRow>
				)}
				{templates.map((template) => (
					<TemplateAccessRow
						key={template.id}
						template={template}
						canEdit={canEdit}
						isSaving={isSaving}
						onToggle={(checked) => onToggle(template.id, checked)}
						onConfigureParameters={() => onConfigureParameters(template.id)}
					/>
				))}
			</SettingsCard>
		</>
	);
};

type TemplateAccessRowProps = {
	readonly template: TemplateRow;
	readonly canEdit: boolean;
	readonly isSaving: boolean;
	readonly onToggle: (checked: boolean) => void;
	readonly onConfigureParameters: () => void;
};

const TemplateAccessRow: React.FC<TemplateAccessRowProps> = ({
	template,
	canEdit,
	isSaving,
	onToggle,
	onConfigureParameters,
}) => (
	<SettingsCardRow className="justify-between">
		<OptionCheckbox
			label={template.label}
			checked={template.isSelected}
			disabled={template.isOnlyAllowed}
			isSaving={isSaving}
			readOnly={!canEdit}
			onCheckedChange={onToggle}
		/>
		<div className="flex shrink-0 items-center gap-2">
			{template.restrictedParameterCount > 0 && (
				<span className="text-xs text-content-secondary">
					{template.restrictedParameterCount === 1
						? "1 parameter restricted"
						: `${template.restrictedParameterCount} parameters restricted`}
				</span>
			)}
			{canEdit && template.isSelected && (
				<Button
					size="sm"
					variant="outline"
					aria-disabled={isSaving}
					className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
					aria-label={`Configure parameters for ${template.label}`}
					onClick={() => {
						if (!isSaving) {
							onConfigureParameters();
						}
					}}
				>
					<SlidersHorizontalIcon />
					Parameters
				</Button>
			)}
		</div>
	</SettingsCardRow>
);

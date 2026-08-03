import type { FC } from "react";
import {
	DetailedError,
	getErrorDetail,
	getErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	type TemplateFilterState,
	TemplatesFilter,
} from "#/pages/TemplatesPage/TemplatesFilter";
import { createDayString } from "#/utils/createDayString";
import { formatTemplateActiveDevelopers } from "#/utils/templates";

const getTemplateLabel = (template: TypesGen.Template) =>
	template.display_name || template.name;

const getTemplateOrganization = (template: TypesGen.Template) =>
	template.organization_display_name || template.organization_name;

interface TemplatesPageViewProps {
	filterState: TemplateFilterState;
	templates: TypesGen.Template[] | undefined;
	isLoading: boolean;
	error: unknown;
	onRetry: () => void;
	onToggleAgentsAllowed: (
		template: TypesGen.Template,
		agentsAllowed: boolean,
	) => void;
	pendingTemplateIDs: ReadonlySet<string>;
	updateErrors: ReadonlyMap<string, unknown>;
}

interface TemplateRowProps {
	template: TypesGen.Template;
	isPending: boolean;
	onToggleAgentsAllowed: (
		template: TypesGen.Template,
		agentsAllowed: boolean,
	) => void;
}

const TemplateRow: FC<TemplateRowProps> = ({
	template,
	isPending,
	onToggleAgentsAllowed,
}) => {
	const label = getTemplateLabel(template);
	const organization = getTemplateOrganization(template);

	return (
		<TableRow>
			<TableCell className="w-full max-w-0 px-4 py-3">
				<div className="flex min-w-0 items-center gap-4">
					<Avatar
						size="lg"
						variant="icon"
						src={template.icon}
						fallback={label}
					/>
					<div className="flex min-w-0 flex-col">
						<span
							className="truncate text-sm font-medium leading-5 text-content-primary"
							title={label}
						>
							{label}
						</span>
						<span
							className="truncate text-sm font-medium leading-5 text-content-secondary"
							title={organization}
						>
							{organization}
						</span>
					</div>
				</div>
			</TableCell>
			<TableCell
				data-pixel="ignore"
				className="whitespace-nowrap text-sm font-medium leading-6 text-content-secondary"
			>
				{createDayString(template.updated_at)}
			</TableCell>
			<TableCell className="whitespace-nowrap text-sm font-medium leading-6 text-content-secondary">
				{`${formatTemplateActiveDevelopers(template.active_user_count)} developer${template.active_user_count === 1 ? "" : "s"}`}
			</TableCell>
			<TableCell className="whitespace-nowrap pr-4 text-right">
				<Switch
					checked={template.agents_allowed}
					onCheckedChange={(agentsAllowed) =>
						onToggleAgentsAllowed(template, agentsAllowed)
					}
					disabled={isPending}
					aria-label={`Allow Coder Agents to create workspaces with ${label} in ${organization}`}
				/>
			</TableCell>
		</TableRow>
	);
};

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	filterState,
	templates,
	isLoading,
	error,
	onRetry,
	onToggleAgentsAllowed,
	pendingTemplateIDs,
	updateErrors,
}) => {
	const hasValidationError = hasError(error) && isApiValidationError(error);

	return (
		<div>
			<SettingsHeader>
				<SettingsHeaderTitle>Templates</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Choose which templates Coder Agents can use to create workspaces.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<TemplatesFilter
				filter={filterState.filter}
				error={error}
				userMenu={filterState.menus.user}
			/>
			{hasError(error) && !hasValidationError ? (
				<div className="flex flex-col gap-4">
					<ErrorAlert
						error={
							new DetailedError(
								"Failed to load templates.",
								getErrorDetail(error),
							)
						}
					/>
					<Button variant="outline" size="sm" type="button" onClick={onRetry}>
						Retry
					</Button>
				</div>
			) : hasValidationError ? null : (
				<>
					<Table
						aria-label="Templates Coder Agents can use to create workspaces"
						className="table-fixed"
					>
						<TableHeader>
							<TableRow>
								<TableHead className="w-1/2">Template</TableHead>
								<TableHead className="w-44">Last updated</TableHead>
								<TableHead className="w-44">Used by</TableHead>
								<TableHead className="w-36 text-right">
									New workspaces
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody size="lg">
							{isLoading || !templates ? (
								<TableLoader />
							) : templates.length === 0 ? (
								<TableEmpty
									message={
										filterState.filter.used
											? "No results matched your search"
											: "No templates found."
									}
									description={
										filterState.filter.used
											? undefined
											: "Create a template before configuring Coder Agents access."
									}
									isCompact
									className="min-h-52"
								/>
							) : (
								templates.map((template) => (
									<TemplateRow
										key={template.id}
										template={template}
										isPending={pendingTemplateIDs.has(template.id)}
										onToggleAgentsAllowed={onToggleAgentsAllowed}
									/>
								))
							)}
						</TableBody>
					</Table>
					{templates
						?.filter((template) => updateErrors.has(template.id))
						.map((template) => (
							<p
								key={template.id}
								role="alert"
								className="m-0 pt-3 text-xs text-content-destructive"
							>
								{`${getTemplateLabel(template)}: ${getErrorMessage(
									updateErrors.get(template.id),
									"Failed to update Coder Agents access.",
								)}`}
							</p>
						))}
				</>
			)}
		</div>
	);
};

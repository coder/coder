import type { FC } from "react";
import { DetailedError, getErrorDetail, getErrorMessage } from "#/api/errors";
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
import { createDayString } from "#/utils/createDayString";
import { formatTemplateActiveDevelopers } from "#/utils/templates";

interface TemplatesPageViewProps {
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
	const label = template.display_name || template.name;
	const organization =
		template.organization_display_name || template.organization_name;

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
					aria-label={`Allow Coder Agents to use ${label} in ${organization}`}
				/>
			</TableCell>
		</TableRow>
	);
};

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	templates,
	isLoading,
	error,
	onRetry,
	onToggleAgentsAllowed,
	pendingTemplateIDs,
	updateErrors,
}) => {
	return (
		<div>
			<SettingsHeader>
				<SettingsHeaderTitle>Templates</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Choose which templates Coder Agents can use to create workspaces.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{error ? (
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
			) : (
				<>
					<Table
						aria-label="Coder Agents template access"
						className="table-fixed"
					>
						<TableHeader>
							<TableRow>
								<TableHead className="w-1/2">Template</TableHead>
								<TableHead className="w-44">Last updated</TableHead>
								<TableHead className="w-44">Used by</TableHead>
								<TableHead className="w-36 text-right">
									Agents allowed
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody size="lg">
							{isLoading ? (
								<TableLoader />
							) : !templates || templates.length === 0 ? (
								<TableEmpty
									message="No templates found."
									description="Create a template before configuring Coder Agents access."
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
								{`${template.display_name || template.name}: ${getErrorMessage(
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

import {
	ChevronDownIcon,
	EllipsisVerticalIcon,
	PlusIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { DetailedError, getErrorDetail, getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
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
	templatesData: TypesGen.Template[] | undefined;
	allowlistData: TypesGen.ChatTemplateAllowlist | undefined;
	isLoading: boolean;
	templatesError: unknown;
	allowlistError: unknown;
	onRetry: () => void;
	onSaveAllowlist: (req: TypesGen.ChatTemplateAllowlist) => void;
	isSaving: boolean;
	saveError: unknown;
}

interface AddTemplatePickerProps {
	availableTemplates: TypesGen.Template[];
	isSaving: boolean;
	onAddTemplate: (templateID: string) => void;
}

const AddTemplatePicker: FC<AddTemplatePickerProps> = ({
	availableTemplates,
	isSaving,
	onAddTemplate,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const filteredTemplates = availableTemplates.filter((template) =>
		`${template.display_name || template.name} ${template.name}`
			.toLowerCase()
			.includes(search.trim().toLowerCase()),
	);

	return (
		<Popover
			open={open}
			onOpenChange={(nextOpen) => {
				setOpen(nextOpen);
				if (!nextOpen) {
					setSearch("");
				}
			}}
		>
			<PopoverTrigger asChild>
				<Button variant="outline" disabled={isSaving}>
					<PlusIcon />
					<span>Add template</span>
					<ChevronDownIcon className="ml-1 size-icon-xs" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				className="w-80 overflow-hidden border-border-default p-0"
			>
				<Command
					shouldFilter={false}
					className="[&_[cmdk-input-wrapper]]:border-0 [&_[cmdk-input-wrapper]]:border-border-default [&_[cmdk-input-wrapper]]:border-b [&_[cmdk-input-wrapper]]:border-solid [&_[cmdk-input-wrapper]]:px-4 [&_[cmdk-input-wrapper]]:py-3"
				>
					<CommandInput
						value={search}
						onValueChange={setSearch}
						placeholder="Search..."
						aria-label="Search templates"
						className="h-auto py-0"
					/>
					<CommandList className="max-h-80 border-t-0">
						<CommandEmpty>No templates found.</CommandEmpty>
						<CommandGroup>
							{filteredTemplates.map((template) => (
								<CommandItem
									key={template.id}
									value={template.id}
									className="gap-3"
									onSelect={() => {
										onAddTemplate(template.id);
										setOpen(false);
									}}
								>
									<Avatar
										size="lg"
										variant="icon"
										src={template.icon}
										fallback={template.display_name || template.name}
									/>
									<span className="min-w-0 truncate">
										{template.display_name || template.name}
									</span>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

interface TemplateRowProps {
	template: TypesGen.Template;
	isSaving: boolean;
	onRemoveTemplate: (templateID: string) => void;
}

const TemplateRow: FC<TemplateRowProps> = ({
	template,
	isSaving,
	onRemoveTemplate,
}) => {
	const label = template.display_name || template.name;

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
						{template.description && (
							<span
								className="truncate text-sm font-medium leading-5 text-content-secondary"
								title={template.description}
							>
								{template.description}
							</span>
						)}
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
			<TableCell className="w-12 whitespace-nowrap pr-4 text-right">
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							type="button"
							disabled={isSaving}
							aria-label={`Actions for ${label}`}
						>
							<EllipsisVerticalIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onSelect={() => onRemoveTemplate(template.id)}
						>
							<TrashIcon />
							Remove
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

interface TemplatesTableProps {
	isLoading: boolean;
	allowlistedTemplates: TypesGen.Template[];
	availableTemplates: TypesGen.Template[];
	isSaving: boolean;
	onAddTemplate: (templateID: string) => void;
	onRemoveTemplate: (templateID: string) => void;
}

const TemplatesTable: FC<TemplatesTableProps> = ({
	isLoading,
	allowlistedTemplates,
	availableTemplates,
	isSaving,
	onAddTemplate,
	onRemoveTemplate,
}) => {
	return (
		<Table aria-label="Allowed templates" className="table-fixed">
			<TableHeader>
				<TableRow>
					<TableHead className="w-1/2">Name</TableHead>
					<TableHead className="w-44">Last updated</TableHead>
					<TableHead className="w-44">Used by</TableHead>
					<TableHead className="w-12">
						<span className="sr-only">Actions</span>
					</TableHead>
				</TableRow>
			</TableHeader>
			<TableBody size="lg">
				{isLoading ? (
					<TableLoader />
				) : allowlistedTemplates.length === 0 ? (
					<TableEmpty
						message="No restrictions set."
						description="All templates are available. Add a template to create an allowlist."
						cta={
							<AddTemplatePicker
								availableTemplates={availableTemplates}
								isSaving={isSaving}
								onAddTemplate={onAddTemplate}
							/>
						}
						isCompact
						className="min-h-52"
					/>
				) : (
					allowlistedTemplates.map((template) => (
						<TemplateRow
							key={template.id}
							template={template}
							isSaving={isSaving}
							onRemoveTemplate={onRemoveTemplate}
						/>
					))
				)}
			</TableBody>
		</Table>
	);
};

export const TemplatesPageView: FC<TemplatesPageViewProps> = ({
	templatesData,
	allowlistData,
	isLoading,
	templatesError,
	allowlistError,
	onRetry,
	onSaveAllowlist,
	isSaving,
	saveError,
}) => {
	const templateIDs = allowlistData?.template_ids ?? [];
	const { allowlistedTemplates, availableTemplates, resolvedTemplateIDs } =
		useMemo(() => {
			const allTemplates = templatesData ?? [];
			const templatesByID = new Map(
				allTemplates.map((template) => [template.id, template]),
			);
			const selectedIDs = new Set(templateIDs);
			const allowlisted = templateIDs
				.map((templateID) => templatesByID.get(templateID))
				.filter((template) => template !== undefined);
			const resolvedIDs = allowlisted.map((template) => template.id);
			const available = allTemplates
				.filter((template) => !selectedIDs.has(template.id))
				.toSorted((left, right) =>
					(left.display_name || left.name).localeCompare(
						right.display_name || right.name,
					),
				);

			return {
				allowlistedTemplates: allowlisted,
				availableTemplates: available,
				resolvedTemplateIDs: resolvedIDs,
			};
		}, [templatesData, templateIDs]);

	const saveTemplateIDs = (nextTemplateIDs: string[]) => {
		onSaveAllowlist({ template_ids: nextTemplateIDs });
	};

	const handleAddTemplate = (templateID: string) => {
		if (resolvedTemplateIDs.includes(templateID)) {
			return;
		}
		saveTemplateIDs([...resolvedTemplateIDs, templateID]);
	};

	const handleRemoveTemplate = (templateID: string) => {
		saveTemplateIDs(resolvedTemplateIDs.filter((id) => id !== templateID));
	};

	const hasTemplatesError = Boolean(templatesError);
	const hasAllowlistError = Boolean(allowlistError);
	const hasError = hasTemplatesError || hasAllowlistError;

	return (
		<div>
			<SettingsHeader
				actions={
					!isLoading &&
					!hasError &&
					allowlistedTemplates.length > 0 && (
						<AddTemplatePicker
							availableTemplates={availableTemplates}
							isSaving={isSaving}
							onAddTemplate={handleAddTemplate}
						/>
					)
				}
			>
				<SettingsHeaderTitle>Templates</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Restrict which templates agents can use to create workspaces.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{hasError ? (
				<div className="flex flex-col gap-4">
					{hasTemplatesError && (
						<ErrorAlert
							error={
								new DetailedError(
									"Failed to load templates.",
									getErrorDetail(templatesError),
								)
							}
						/>
					)}
					{hasAllowlistError && (
						<ErrorAlert
							error={
								new DetailedError(
									"Failed to load template allowlist configuration.",
									getErrorDetail(allowlistError),
								)
							}
						/>
					)}
					<Button variant="outline" size="sm" type="button" onClick={onRetry}>
						Retry
					</Button>
				</div>
			) : (
				<>
					<TemplatesTable
						isLoading={isLoading}
						allowlistedTemplates={allowlistedTemplates}
						availableTemplates={availableTemplates}
						isSaving={isSaving}
						onAddTemplate={handleAddTemplate}
						onRemoveTemplate={handleRemoveTemplate}
					/>
					{saveError && (
						<p
							role="alert"
							className="m-0 pt-3 text-xs text-content-destructive"
						>
							{getErrorMessage(saveError, "Failed to save template allowlist.")}
						</p>
					)}
				</>
			)}
		</div>
	);
};

import {
	ChevronDownIcon,
	EllipsisVerticalIcon,
	PlusIcon,
	SearchIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { DetailedError, getErrorDetail } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";

import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { EmptyState } from "#/components/EmptyState/EmptyState";
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
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { cn } from "#/utils/cn";
import { createDayString } from "#/utils/createDayString";
import { formatTemplateActiveDevelopers } from "#/utils/templates";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface TemplatesPageViewProps {
	templatesData: TypesGen.Template[] | undefined;
	allowlistData: TypesGen.ChatTemplateAllowlist | undefined;
	isLoading: boolean;
	templatesError: unknown;
	allowlistError: unknown;
	onRetry: () => void;
	onSaveAllowlist: (
		req: TypesGen.ChatTemplateAllowlist,
		options?: MutationCallbacks,
	) => void;
	isSaving: boolean;
	isSaveError: boolean;
}

interface AddTemplateDropdownProps {
	availableTemplates: TypesGen.Template[];
	isSaving: boolean;
	onAddTemplate: (templateID: string) => void;
}

const AddTemplateDropdown: FC<AddTemplateDropdownProps> = ({
	availableTemplates,
	isSaving,
	onAddTemplate,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const normalizedSearch = search.trim().toLowerCase();
	const filteredTemplates = availableTemplates.filter((template) => {
		if (!normalizedSearch) {
			return true;
		}

		return `${template.display_name || template.name} ${template.name}`
			.toLowerCase()
			.includes(normalizedSearch);
	});

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
				<div className="border-0 border-border-default border-b border-solid px-4 py-3">
					<div className="flex items-center gap-2.5">
						<SearchIcon className="size-icon-sm shrink-0 text-content-secondary" />
						<input
							value={search}
							onChange={(event) => setSearch(event.target.value)}
							placeholder="Search..."
							aria-label="Search templates"
							className="min-w-0 flex-1 border-none bg-transparent p-0 text-sm text-content-primary outline-none placeholder:text-content-secondary"
						/>
					</div>
				</div>
				<div className="flex max-h-80 flex-col items-start overflow-y-auto p-2">
					{filteredTemplates.length === 0 ? (
						<div className="w-full px-2 py-6 text-center text-sm text-content-secondary">
							No templates found.
						</div>
					) : (
						filteredTemplates.map((template) => (
							<button
								key={template.id}
								type="button"
								className="flex w-full cursor-pointer items-center gap-3 rounded-sm border-none bg-transparent px-2 py-2 text-left text-sm font-medium text-content-secondary outline-none hover:bg-surface-secondary hover:text-content-primary focus:bg-surface-secondary focus:text-content-primary"
								onClick={() => {
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
							</button>
						))
					)}
				</div>
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
		<TableRow className="h-[72px]">
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
				data-chromatic="ignore"
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
			<TableBody>
				{isLoading ? (
					<TableLoader />
				) : allowlistedTemplates.length === 0 ? (
					<TableRow>
						<TableCell colSpan={999} className="p-0!">
							<EmptyState
								message="No restrictions set."
								description="All templates are available. Add a template to create an allowlist."
								cta={
									<AddTemplateDropdown
										availableTemplates={availableTemplates}
										isSaving={isSaving}
										onAddTemplate={onAddTemplate}
									/>
								}
								isCompact
								className="min-h-52"
							/>
						</TableCell>
					</TableRow>
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
	isSaveError,
}) => {
	const templateIDs = allowlistData?.template_ids ?? [];
	const { allowlistedTemplates, availableTemplates } = useMemo(() => {
		const allTemplates = templatesData ?? [];
		const templatesByID = new Map(
			allTemplates.map((template) => [template.id, template]),
		);
		const selectedIDs = new Set(templateIDs);
		const allowlisted = templateIDs
			.map((templateID) => templatesByID.get(templateID))
			.filter((template) => template !== undefined);
		const available = allTemplates
			.filter((template) => !selectedIDs.has(template.id))
			.toSorted((left, right) =>
				(left.display_name || left.name).localeCompare(
					right.display_name || right.name,
				),
			);

		return { allowlistedTemplates: allowlisted, availableTemplates: available };
	}, [templatesData, templateIDs]);

	const saveTemplateIDs = (nextTemplateIDs: string[]) => {
		onSaveAllowlist({ template_ids: nextTemplateIDs });
	};

	const handleAddTemplate = (templateID: string) => {
		if (templateIDs.includes(templateID)) {
			return;
		}
		saveTemplateIDs([...templateIDs, templateID]);
	};

	const handleRemoveTemplate = (templateID: string) => {
		saveTemplateIDs(templateIDs.filter((id) => id !== templateID));
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
						<AddTemplateDropdown
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
					{isSaveError && (
						<p
							role="alert"
							className={cn("m-0 pt-3 text-xs text-content-destructive")}
						>
							Failed to save template allowlist.
						</p>
					)}
				</>
			)}
		</div>
	);
};

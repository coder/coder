import { ChevronDownIcon, PlusIcon, SearchIcon } from "lucide-react";
import { type FC, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	canManageProviderModels,
	type ProviderState,
} from "#/modules/aiModels/providerStates";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { paginateItems } from "#/utils/paginateItems";
import { ModelRow } from "./components/ModelRow";

const MODELS_PAGE_SIZE = 10;
const ALL_PROVIDERS_VALUE = "all";

const AddModelDropdown: FC<{
	providerStates: readonly ProviderState[];
	align?: "start" | "end";
}> = ({ providerStates, align = "end" }) => {
	const navigate = useNavigate();
	const manageableProviderStates = providerStates.filter(
		canManageProviderModels,
	);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline">
					<PlusIcon />
					<span>Add model</span>
					<ChevronDownIcon className="ml-1 size-icon-xs" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align={align} className="min-w-56">
				<div className="px-2 py-1.5 text-xs font-medium text-content-secondary">
					Select a provider
				</div>
				{manageableProviderStates.length === 0 ? (
					<DropdownMenuItem disabled>No providers available</DropdownMenuItem>
				) : (
					manageableProviderStates.map((providerState) => (
						<DropdownMenuItem
							key={providerState.key}
							onSelect={() =>
								void navigate(
									`/ai/settings/models/add?provider=${encodeURIComponent(
										providerState.key,
									)}`,
								)
							}
						>
							<ProviderIcon provider={providerState.provider} />
							<span>{providerState.label}</span>
						</DropdownMenuItem>
					))
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

interface ModelsPageViewProps {
	isLoading: boolean;
	error: unknown;
	models: readonly ChatModelConfig[];
	providerStates: readonly ProviderState[];
	providerTypeByID: ReadonlyMap<string, string>;
}

const ModelsPageView: FC<ModelsPageViewProps> = ({
	isLoading,
	error,
	models,
	providerStates,
	providerTypeByID,
}) => {
	const navigate = useNavigate();
	const [page, setPage] = useState(1);
	const [searchQuery, setSearchQuery] = useState("");
	const [providerFilter, setProviderFilter] =
		useState<string>(ALL_PROVIDERS_VALUE);

	const providerKeyByModelId = useMemo(() => {
		const map = new Map<string, string>();
		for (const providerState of providerStates) {
			for (const providerModel of providerState.modelConfigs) {
				map.set(providerModel.id, providerState.key);
			}
		}
		return map;
	}, [providerStates]);

	const providerLabelByModelId = useMemo(() => {
		const map = new Map<string, string>();
		for (const providerState of providerStates) {
			for (const providerModel of providerState.modelConfigs) {
				map.set(providerModel.id, providerState.label);
			}
		}
		return map;
	}, [providerStates]);

	const filteredModels = useMemo(() => {
		const normalizedQuery = searchQuery.trim().toLowerCase();
		return models.filter((model) => {
			if (
				providerFilter !== ALL_PROVIDERS_VALUE &&
				providerKeyByModelId.get(model.id) !== providerFilter
			) {
				return false;
			}
			if (normalizedQuery.length === 0) {
				return true;
			}
			const haystack = [
				model.display_name,
				model.model,
				providerLabelByModelId.get(model.id) ?? "",
			]
				.join(" ")
				.toLowerCase();
			return haystack.includes(normalizedQuery);
		});
	}, [
		models,
		providerFilter,
		providerKeyByModelId,
		providerLabelByModelId,
		searchQuery,
	]);

	const { pagedItems, clampedPage, hasPreviousPage, hasNextPage } =
		paginateItems(filteredModels, MODELS_PAGE_SIZE, page);

	const hasModels = models.length > 0;
	const hasFilters =
		searchQuery.trim().length > 0 || providerFilter !== ALL_PROVIDERS_VALUE;

	const handleSearchChange = (value: string) => {
		setSearchQuery(value);
		setPage(1);
	};

	const handleProviderChange = (value: string) => {
		setProviderFilter(value);
		setPage(1);
	};

	return (
		<div>
			<SettingsHeader
				actions={<AddModelDropdown providerStates={providerStates} />}
			>
				<SettingsHeaderTitle>Models</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Choose which models from your configured providers are available for
					users to select. You can set a default and adjust context limits.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && (
				<div className="mb-4">
					<ErrorAlert error={error} />
				</div>
			)}
			<div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center">
				<div className="flex-1">
					<InputGroup>
						<InputGroupAddon>
							<SearchIcon />
						</InputGroupAddon>
						<InputGroupInput
							type="search"
							placeholder="Search models..."
							aria-label="Search models"
							value={searchQuery}
							onChange={(e) => handleSearchChange(e.target.value)}
						/>
					</InputGroup>
				</div>
				<Select value={providerFilter} onValueChange={handleProviderChange}>
					<SelectTrigger
						className="w-full shadow-none sm:w-60"
						aria-label="Filter by provider"
					>
						<SelectValue placeholder="All providers" />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value={ALL_PROVIDERS_VALUE}>All providers</SelectItem>
						{providerStates.map((providerState) => (
							<SelectItem key={providerState.key} value={providerState.key}>
								<span className="flex items-center gap-2">
									<ProviderIcon provider={providerState.provider} />
									{providerState.label}
								</span>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</div>
			<Table className="table-fixed" aria-label="Models">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/4">Provider</TableHead>
						<TableHead className="w-1/4">Context limit</TableHead>
						<TableHead className="w-40">Status</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Open model</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">
					{isLoading ? (
						<TableLoader />
					) : !hasModels ? (
						<TableEmpty
							message="No models configured"
							description="Configured models will appear here."
							cta={
								<AddModelDropdown
									providerStates={providerStates}
									align="start"
								/>
							}
						/>
					) : filteredModels.length === 0 ? (
						<TableEmpty
							message="No models match your filters"
							description="Try a different search term or provider."
						/>
					) : (
						pagedItems.map((model) => (
							<ModelRow
								key={model.id}
								model={model}
								providerLabel={providerLabelByModelId.get(model.id) ?? ""}
								providerTypeByID={providerTypeByID}
								onClick={() => void navigate(`/ai/settings/models/${model.id}`)}
							/>
						))
					)}
				</TableBody>
			</Table>
			{filteredModels.length > 0 && (
				<div className="flex items-center justify-between pt-4">
					<div className="flex-1">
						<PaginationWidgetBase
							currentPage={clampedPage}
							pageSize={MODELS_PAGE_SIZE}
							totalRecords={filteredModels.length}
							onPageChange={setPage}
							hasPreviousPage={hasPreviousPage}
							hasNextPage={hasNextPage}
						/>
					</div>
					<span className="text-xs text-content-secondary">
						Showing <strong className="font-medium">{pagedItems.length}</strong>{" "}
						of <strong className="font-medium">{filteredModels.length}</strong>{" "}
						models
						{hasFilters && (
							<>
								{" "}
								(filtered from{" "}
								<strong className="font-medium">{models.length}</strong>)
							</>
						)}
					</span>
				</div>
			)}
		</div>
	);
};

export default ModelsPageView;

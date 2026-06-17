import { ChevronDownIcon, PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
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
import { Link } from "#/components/Link/Link";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
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

interface ModelsPageViewProps {
	isLoading: boolean;
	error: unknown;
	models: readonly ChatModelConfig[];
	providerStates: readonly ProviderState[];
}

const AddModelDropdown: FC<{
	manageableProviders: readonly ProviderState[];
	align?: "start" | "end";
}> = ({ manageableProviders, align = "end" }) => {
	const navigate = useNavigate();
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
				{manageableProviders.map((providerState) => (
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
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const ModelsPageView: FC<ModelsPageViewProps> = ({
	isLoading,
	error,
	models,
	providerStates,
}) => {
	const navigate = useNavigate();
	const [page, setPage] = useState(1);
	const manageableProviders = providerStates.filter(canManageProviderModels);
	const hasManageableProvider = manageableProviders.length > 0;

	const { pagedItems, clampedPage, hasPreviousPage, hasNextPage } =
		paginateItems(models, MODELS_PAGE_SIZE, page);

	// Map each model to its provider's display label for the Provider column.
	const providerLabelByModelId = new Map<string, string>();
	for (const providerState of providerStates) {
		for (const providerModel of providerState.modelConfigs) {
			providerLabelByModelId.set(providerModel.id, providerState.label);
		}
	}

	return (
		<div>
			<SettingsHeader
				actions={
					hasManageableProvider ? (
						<AddModelDropdown manageableProviders={manageableProviders} />
					) : undefined
				}
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
				<TableBody>
					{isLoading ? (
						<TableLoader />
					) : models.length === 0 ? (
						<TableEmpty
							message="No models configured"
							description={
								hasManageableProvider
									? "Add a model from one of your configured providers."
									: undefined
							}
							cta={
								hasManageableProvider ? (
									<AddModelDropdown
										manageableProviders={manageableProviders}
										align="start"
									/>
								) : (
									<p className="text-sm text-content-secondary m-0">
										Connect a <Link href="/ai/settings">provider</Link> first to
										add models.
									</p>
								)
							}
						/>
					) : (
						pagedItems.map((model) => (
							<ModelRow
								key={model.id}
								model={model}
								providerLabel={providerLabelByModelId.get(model.id) ?? ""}
								onClick={() => void navigate(`/ai/settings/models/${model.id}`)}
							/>
						))
					)}
				</TableBody>
			</Table>
			{models.length > 0 && (
				<div className="flex items-center justify-between pt-4">
					<div className="flex-1">
						<PaginationWidgetBase
							currentPage={clampedPage}
							pageSize={MODELS_PAGE_SIZE}
							totalRecords={models.length}
							onPageChange={setPage}
							hasPreviousPage={hasPreviousPage}
							hasNextPage={hasNextPage}
						/>
					</div>
					<span className="text-xs text-content-secondary">
						Showing <strong className="font-medium">{pagedItems.length}</strong>{" "}
						of <strong className="font-medium">{models.length}</strong> models
					</span>
				</div>
			)}
		</div>
	);
};

export default ModelsPageView;

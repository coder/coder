import { type FC, useState } from "react";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import type { ProviderState } from "#/modules/aiModels/providerStates";
import { paginateItems } from "#/utils/paginateItems";
import { ModelRow } from "./components/ModelRow";

const MODELS_PAGE_SIZE = 10;

interface ModelsPageViewProps {
	isLoading: boolean;
	error: unknown;
	models: readonly ChatModelConfig[];
	providerStates: readonly ProviderState[];
}

const ModelsPageView: FC<ModelsPageViewProps> = ({
	isLoading,
	error,
	models,
	providerStates,
}) => {
	const [page, setPage] = useState(1);
	const { pagedItems, clampedPage, hasPreviousPage, hasNextPage } =
		paginateItems(models, MODELS_PAGE_SIZE, page);

	const providerLabelByModelId = new Map<string, string>();
	for (const providerState of providerStates) {
		for (const providerModel of providerState.modelConfigs) {
			providerLabelByModelId.set(providerModel.id, providerState.label);
		}
	}

	return (
		<div>
			<SettingsHeader>
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
					</TableRow>
				</TableHeader>
				<TableBody>
					{isLoading ? (
						<TableLoader />
					) : models.length === 0 ? (
						<TableEmpty
							message="No models configured"
							description="Configured models will appear here."
						/>
					) : (
						pagedItems.map((model) => (
							<ModelRow
								key={model.id}
								model={model}
								providerLabel={providerLabelByModelId.get(model.id) ?? ""}
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

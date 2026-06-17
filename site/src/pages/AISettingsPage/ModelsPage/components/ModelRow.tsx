import type { FC } from "react";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";

type ModelRowProps = {
	model: ChatModelConfig;
	providerLabel: string;
};

const formatContextLimit = (contextLimit: number): string => {
	if (!Number.isFinite(contextLimit)) {
		return "N/A";
	}
	return `${contextLimit.toLocaleString("en-US")} tokens`;
};

export const ModelRow: FC<ModelRowProps> = ({ model, providerLabel }) => {
	const displayName = model.display_name || model.model;

	return (
		<TableRow className="h-[72px]">
			<TableCell className="min-w-0 p-4">
				<div className="flex min-w-0 items-center gap-4">
					<Avatar
						size="lg"
						className="flex shrink-0 items-center justify-center"
					>
						<ProviderIcon provider={model.provider} />
					</Avatar>
					<span
						className="truncate text-sm font-medium leading-6 text-content-primary"
						title={displayName}
					>
						{displayName}
					</span>
				</div>
			</TableCell>
			<TableCell className="min-w-0">
				<span
					className="block truncate text-sm font-medium leading-6 text-content-secondary"
					title={providerLabel}
				>
					{providerLabel || "N/A"}
				</span>
			</TableCell>
			<TableCell className="min-w-0">
				<span className="block truncate text-sm font-medium leading-6 text-content-secondary">
					{formatContextLimit(model.context_limit)}
				</span>
			</TableCell>
			<TableCell>
				<div className="flex flex-wrap items-center gap-2">
					{model.is_default && <Badge variant="default">Default</Badge>}
					<Badge variant="default">
						{model.enabled ? "Enabled" : "Disabled"}
					</Badge>
				</div>
			</TableCell>
		</TableRow>
	);
};

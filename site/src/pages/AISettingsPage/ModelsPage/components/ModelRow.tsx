import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";

type ModelRowProps = {
	model: ChatModelConfig;
	providerLabel: string;
	providerTypeByID: ReadonlyMap<string, string>;
	hasProvider: boolean;
	providerEnabled: boolean;
	onClick: () => void;
};

const formatContextLimit = (contextLimit: number): string => {
	if (!Number.isFinite(contextLimit)) {
		return "N/A";
	}
	return `${contextLimit.toLocaleString("en-US")} tokens`;
};

export const ModelRow: FC<ModelRowProps> = ({
	model,
	providerLabel,
	providerTypeByID,
	hasProvider,
	providerEnabled,
	onClick,
}) => {
	const clickableProps = useClickableTableRow({ onClick });
	const displayName = model.display_name || model.model;
	// Models whose provider is missing or disabled cannot be used, so the
	// status column reflects that regardless of the persisted enabled flag.
	const isEffectivelyEnabled = model.enabled && hasProvider && providerEnabled;

	return (
		<TableRow {...clickableProps}>
			<TableCell className="min-w-0 px-4 py-3">
				<div className="flex min-w-0 items-center gap-4">
					<Avatar
						size="lg"
						className="flex shrink-0 items-center justify-center"
					>
						<ProviderIcon
							provider={providerTypeByID.get(model.ai_provider_id) ?? ""}
						/>
					</Avatar>
					<div className="flex min-w-0 items-center gap-2">
						<span
							className="truncate text-sm font-medium leading-6 text-content-primary"
							title={displayName}
						>
							{displayName}
						</span>
						{model.is_default && (
							<Badge variant="default" className="shrink-0">
								Default
							</Badge>
						)}
					</div>
				</div>
			</TableCell>
			<TableCell className="min-w-0">
				{hasProvider ? (
					<span
						className="block truncate text-sm font-medium leading-6 text-content-secondary"
						title={providerLabel}
					>
						{providerLabel}
					</span>
				) : (
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="block w-fit truncate text-sm font-medium leading-6 text-content-secondary underline decoration-dashed underline-offset-4">
								Unset
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom">
							The provider connected to this model has been deleted.
						</TooltipContent>
					</Tooltip>
				)}
			</TableCell>
			<TableCell className="min-w-0">
				<span className="block truncate text-sm font-medium leading-6 text-content-secondary">
					{formatContextLimit(model.context_limit)}
				</span>
			</TableCell>
			<TableCell>
				<Badge variant="default">
					{isEffectivelyEnabled ? "Enabled" : "Disabled"}
				</Badge>
			</TableCell>
			<TableCell className="w-10 text-center">
				<div className="flex justify-end items-center gap-8 pr-4">
					<ChevronRightIcon
						aria-hidden
						className="size-icon-md text-content-primary flex-shrink-0"
					/>
				</div>
			</TableCell>
		</TableRow>
	);
};

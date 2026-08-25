import { ChevronRightIcon, InfoIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatModel } from "#/api/typesGenerated";
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
import { cn } from "#/utils/cn";

type ModelRowProps = {
	model: ChatModel;
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
	// status cell surfaces that regardless of the persisted enabled flag.
	const providerNotice = !hasProvider
		? "The provider connected to this model has been deleted."
		: !providerEnabled
			? "The provider connected to this model is disabled."
			: null;

	// Keep tooltip activation from triggering the clickable row's navigation.
	const stopPropagation = (event: React.SyntheticEvent) => {
		event.stopPropagation();
	};

	return (
		<TableRow {...clickableProps}>
			<TableCell className="min-w-0 px-4 py-3">
				<div className="flex min-w-0 items-center gap-4">
					<Avatar
						size="lg"
						className={cn(
							"flex shrink-0 items-center justify-center",
							!model.enabled && "opacity-50",
						)}
					>
						<ProviderIcon
							provider={providerTypeByID.get(model.ai_provider_id) ?? ""}
						/>
					</Avatar>
					<div className="flex min-w-0 items-center gap-2">
						<span
							className={cn(
								"truncate text-sm font-medium leading-6",
								model.enabled
									? "text-content-primary"
									: "text-content-secondary",
							)}
							title={displayName}
						>
							{displayName}
						</span>
						{model.is_default && (
							<Badge variant="default" className="shrink-0">
								Default
							</Badge>
						)}
						{!model.enabled && (
							<Badge variant="default" className="shrink-0">
								Disabled
							</Badge>
						)}
						{providerNotice && (
							<Tooltip>
								<TooltipTrigger asChild>
									<Badge
										variant="warning"
										className="shrink-0"
										tabIndex={0}
										aria-label={`Unavailable: ${providerNotice}`}
										onClick={stopPropagation}
										onKeyDown={stopPropagation}
										onKeyUp={stopPropagation}
									>
										Unavailable
									</Badge>
								</TooltipTrigger>
								<TooltipContent side="bottom" className="max-w-[240px]">
									{providerNotice}
								</TooltipContent>
							</Tooltip>
						)}
					</div>
				</div>
			</TableCell>
			<TableCell className="min-w-0">
				{hasProvider ? (
					<span
						className={cn(
							"block truncate text-sm font-medium leading-6",
							model.enabled
								? "text-content-secondary"
								: "text-content-disabled",
						)}
						title={providerLabel}
					>
						{providerLabel}
					</span>
				) : (
					<div className="flex items-center gap-1">
						<span className="truncate text-sm font-medium leading-6 text-content-secondary">
							Unset
						</span>
						<Tooltip>
							<TooltipTrigger asChild>
								<InfoIcon
									aria-label="Provider status"
									className="size-3 text-content-secondary"
								/>
							</TooltipTrigger>
							<TooltipContent side="bottom" className="max-w-[240px]">
								The provider connected to this model has been deleted.
							</TooltipContent>
						</Tooltip>
					</div>
				)}
			</TableCell>
			<TableCell className="min-w-0">
				<span
					className={cn(
						"block truncate text-sm font-medium leading-6",
						model.enabled ? "text-content-secondary" : "text-content-disabled",
					)}
				>
					{formatContextLimit(model.context_limit)}
				</span>
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

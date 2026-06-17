import { ChevronRightIcon } from "lucide-react";
import type { FC } from "react";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { cn } from "#/utils/cn";

type ModelRowProps = {
	model: ChatModelConfig;
	providerLabel: string;
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
	onClick,
}) => {
	const clickableProps = useClickableTableRow({ onClick });
	const displayName = model.display_name || model.model;

	return (
		<TableRow
			{...clickableProps}
			className={cn(clickableProps.className, "h-[72px]")}
		>
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

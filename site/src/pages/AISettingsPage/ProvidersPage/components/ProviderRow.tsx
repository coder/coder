import { ChevronRightIcon } from "lucide-react";
import {
	AgentsUnsupportedProviderTypes,
	type AIProvider,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { ProviderIcon } from "./ProviderIcon";
import { getProviderDisplayType } from "./providerFormApiMap";

type ProviderRowProps = {
	provider: AIProvider;
	onClick?: () => void;
};

export const ProviderRow: React.FC<ProviderRowProps> = ({
	provider,
	onClick,
}) => {
	const clickableProps = useClickableTableRow({
		onClick: () => onClick?.(),
	});
	const displayName = provider.display_name || provider.name;

	return (
		<TableRow key={provider.name} {...clickableProps}>
			<TableCell className="min-w-0 px-4 py-3">
				<AvatarData
					title={displayName}
					avatar={
						<Avatar
							size="lg"
							className="flex shrink-0 items-center justify-center"
						>
							<ProviderIcon
								provider={getProviderDisplayType(provider)}
								icon={provider.icon}
							/>
						</Avatar>
					}
				/>
			</TableCell>
			<TableCell className="min-w-0">
				<span
					className="block truncate text-content-secondary"
					title={provider.base_url}
				>
					{provider.base_url}
				</span>
			</TableCell>
			<TableCell>
				<div className="flex flex-wrap items-center gap-1">
					{provider.enabled && <Badge variant="default">Enabled</Badge>}
					{AgentsUnsupportedProviderTypes.some((t) => t === provider.type) && (
						<Badge
							variant="info"
							title="This provider works with the AI Gateway proxy but Coder Agents can't use it."
						>
							Not supported in Agents
						</Badge>
					)}
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

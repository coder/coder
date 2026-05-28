import { ChevronRightIcon } from "lucide-react";
import type { AIProvider } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Badge } from "#/components/Badge/Badge";
import { TableCell, TableRow } from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { relativeTime } from "#/utils/time";
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
			<TableCell className="min-w-0">
				<AvatarData
					title={displayName}
					avatar={
						<Avatar className="flex shrink-0 items-center justify-center">
							<ProviderIcon provider={getProviderDisplayType(provider)} />
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
				{provider.enabled && <Badge variant="default">Enabled</Badge>}
			</TableCell>
			<TableCell data-chromatic="ignore">
				<LastModifiedCell provider={provider} />
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

const LastModifiedCell: React.FC<{ provider: AIProvider }> = ({ provider }) => {
	const time = relativeTime(provider.updated_at);
	const user = provider.updated_by;

	if (!user) {
		return <span className="text-content-secondary text-sm">{time}</span>;
	}

	return (
		<span className="text-content-secondary text-sm" title={user.username}>
			{time} by {user.name || user.username}
		</span>
	);
};

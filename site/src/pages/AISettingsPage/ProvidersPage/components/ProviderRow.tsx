import { ChevronRightIcon } from "lucide-react";
import type { AIProvider } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { TableCell, TableRow } from "#/components/Table/Table";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
import { ProviderIcon } from "./ProviderIcon";

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
	return (
		<TableRow key={provider.name} {...clickableProps}>
			<TableCell>
				<AvatarData
					title={provider.display_name}
					subtitle={provider.name}
					avatar={
						<Avatar className="flex items-center justify-center">
							<ProviderIcon provider={provider.type} />
						</Avatar>
					}
				/>
			</TableCell>
			<TableCell>
				<div className="flex justify-between">
					<span>{provider.base_url}</span>
					<ChevronRightIcon className="size-icon-sm" />
				</div>
			</TableCell>
		</TableRow>
	);
};

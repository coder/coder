import { CheckIcon, ChevronRightIcon, XIcon } from "lucide-react";
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
	const displayName = provider.display_name || provider.name;

	return (
		<TableRow key={provider.name} {...clickableProps}>
			<TableCell className="min-w-0">
				<AvatarData
					title={displayName}
					subtitle={provider.name}
					avatar={
						<Avatar className="flex shrink-0 items-center justify-center">
							<ProviderIcon provider={provider.type} />
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
			<TableCell className="w-20 text-center">
				<div className="flex justify-end items-center gap-8 pr-4">
					{provider.enabled ? (
						<CheckIcon
							aria-label="Enabled"
							className="inline size-icon-md text-content-success flex-shrink-0"
						/>
					) : (
						<XIcon
							aria-label="Disabled"
							className="inline size-icon-md text-content-destructive flex-shrink-0"
						/>
					)}
					<ChevronRightIcon
						aria-hidden
						className="size-icon-md text-content-primary flex-shrink-0"
					/>
				</div>
			</TableCell>
		</TableRow>
	);
};

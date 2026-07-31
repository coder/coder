import { EllipsisVerticalIcon, PencilIcon, TrashIcon } from "lucide-react";
import type { FC } from "react";
import type { BannerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Switch } from "#/components/Switch/Switch";
import { TableCell, TableRow } from "#/components/Table/Table";

interface AnnouncementBannerItemProps {
	enabled: boolean;
	backgroundColor?: string;
	message?: string;
	onUpdate: (banner: Partial<BannerConfig>) => Promise<void>;
	onEdit: () => void;
	onDelete: () => void;
}

export const AnnouncementBannerItem: FC<AnnouncementBannerItemProps> = ({
	enabled,
	backgroundColor = "#004852",
	message,
	onUpdate,
	onEdit,
	onDelete,
}) => {
	return (
		<TableRow>
			<TableCell className="align-middle pl-5">
				<Switch
					checked={enabled}
					aria-label="Enabled"
					onCheckedChange={(checked) => {
						void onUpdate({ enabled: checked });
					}}
				/>
			</TableCell>

			<TableCell className={!enabled ? "text-content-disabled" : ""}>
				{message || <em>No message</em>}
			</TableCell>

			<TableCell>
				<div className="size-6 rounded-sm" style={{ backgroundColor }} />
			</TableCell>

			<TableCell>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button size="icon-lg" variant="subtle" aria-label="Open menu">
							<EllipsisVerticalIcon aria-hidden="true" />
							<span className="sr-only">Open menu</span>
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem onClick={() => onEdit()}>
							<PencilIcon className="size-icon-xs" />
							Edit&hellip;
						</DropdownMenuItem>
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onClick={() => onDelete()}
						>
							<TrashIcon className="size-icon-xs" />
							Delete&hellip;
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

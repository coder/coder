import Checkbox from "@mui/material/Checkbox";
import { EllipsisVerticalIcon } from "lucide-react";
import type { FC } from "react";
import type { BannerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
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
			<TableCell>
				<Checkbox
					size="small"
					checked={enabled}
					onClick={() => void onUpdate({ enabled: !enabled })}
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
							Edit&hellip;
						</DropdownMenuItem>
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onClick={() => onDelete()}
						>
							Delete&hellip;
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

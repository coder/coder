import type { Interpolation, Theme } from "@emotion/react";
import Checkbox from "@mui/material/Checkbox";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import type { BannerConfig } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "components/DropdownMenu/DropdownMenu";
import { EllipsisVertical } from "lucide-react";
import type { FC } from "react";

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

			<TableCell css={!enabled && styles.disabled}>
				{message || <em>No message</em>}
			</TableCell>

			<TableCell>
				<div css={styles.colorSample} style={{ backgroundColor }}></div>
			</TableCell>

			<TableCell>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							size="icon-lg"
							variant="subtle"
							aria-label="Open menu"
						>
							<EllipsisVertical aria-hidden="true" />
							<span className="sr-only">Open menu</span>
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem
							onClick={() => onEdit()}
						>
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

const styles = {
	disabled: (theme) => ({
		color: theme.roles.inactive.fill.outline,
	}),

	colorSample: {
		width: 24,
		height: 24,
		borderRadius: 4,
	},
} satisfies Record<string, Interpolation<Theme>>;

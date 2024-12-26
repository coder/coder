import { useTheme } from "@emotion/react";
import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { type FC, useState } from "react";
import { UserDropdownContent } from "./UserDropdownContent";

export interface UserDropdownProps {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
}

export const UserDropdown: FC<UserDropdownProps> = ({
	buildInfo,
	user,
	supportLinks,
	onSignOut,
}) => {
	const theme = useTheme();
	const [open, setOpen] = useState(false);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger>
				<button
					type="button"
					className="bg-transparent border-0 cursor-pointer p-0"
				>
					<Avatar fallback={user.username} src={user.avatar_url} size="lg" />
				</button>
			</PopoverTrigger>

			<PopoverContent
				horizontal="right"
				css={{
					".MuiPaper-root": {
						minWidth: "auto",
						width: 260,
						boxShadow: theme.shadows[6],
					},
				}}
			>
				<UserDropdownContent
					user={user}
					buildInfo={buildInfo}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
				/>
			</PopoverContent>
		</Popover>
	);
};

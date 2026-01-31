import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import type { FC } from "react";
import { UserDropdownContent } from "./UserDropdownContent";

interface UserDropdownProps {
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
}

export const UserDropdown: FC<UserDropdownProps> = ({
	buildInfo,
	user,
	supportLinks,
	onSignOut,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					className="bg-transparent border-0 cursor-pointer p-0"
				>
					<Avatar fallback={user.username} src={user.avatar_url} size="lg" />
				</button>
			</DropdownMenuTrigger>

			<DropdownMenuContent align="end" className="min-w-auto w-[260px]">
				<UserDropdownContent
					user={user}
					buildInfo={buildInfo}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

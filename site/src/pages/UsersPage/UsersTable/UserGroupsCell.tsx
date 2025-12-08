import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import type { Group } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { OverflowY } from "components/OverflowY/OverflowY";
import { TableCell } from "components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { UsersIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

type GroupsCellProps = {
	userGroups: readonly Group[] | undefined;
};

export const UserGroupsCell: FC<GroupsCellProps> = ({ userGroups }) => {
	return (
		<TableCell>
			{userGroups === undefined ? (
				// Felt right to add emphasis to the undefined state for semantics
				// ("hey, this isn't normal"), but the default italics looked weird in
				// the table UI
				<em className="not-italic">N/A</em>
			) : (
				<TooltipProvider>
					<Tooltip delayDuration={0}>
						<TooltipTrigger asChild>
							<button
								className="cursor-pointer bg-transparent border-none p-0 color-inherit leading-none"
								type="button"
							>
								<div className="flex flex-row gap-2 items-center">
									<UsersIcon
										className={cn([
											"size-4 opacity-50",
											userGroups.length > 0 && "opacity-80",
										])}
									/>

									<span>
										{userGroups.length} Group{userGroups.length !== 1 && "s"}
									</span>
								</div>
							</button>
						</TooltipTrigger>

						<TooltipContent className="p-0 bg-surface-secondary border-surface-quaternary text-white">
							<OverflowY maxHeight={400}>
								<List
									component="ul"
									className="flex flex-col flex-nowrap py-1 px-0.5 gap-0 text-sm leading-tight"
								>
									{userGroups.map((group) => {
										const groupName = group.display_name || group.name;
										return (
											<ListItem
												key={group.id}
												className="gap-x-2.5 items-center"
											>
												<Avatar
													size="sm"
													variant="icon"
													src={group.avatar_url}
													fallback={groupName}
												/>

												<span className="whitespace-nowrap text-ellipsis overflow-hidden leading-none m-0">
													{groupName || <em>N/A</em>}
												</span>
											</ListItem>
										);
									})}
								</List>
							</OverflowY>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			)}
		</TableCell>
	);
};

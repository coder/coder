import { useTheme } from "@emotion/react";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import { UsersIcon } from "lucide-react";
import type { FC } from "react";
import type { Group } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { OverflowY } from "#/components/OverflowY/OverflowY";
import { TableCell } from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

type GroupsCellProps = {
	userGroups: readonly Group[] | undefined;
};

export const UserGroupsCell: FC<GroupsCellProps> = ({ userGroups }) => {
	const theme = useTheme();

	return (
		<TableCell>
			{userGroups === undefined ? (
				<span>No groups</span>
			) : (
				<TooltipProvider>
					<Tooltip delayDuration={0}>
						<TooltipTrigger asChild>
							<button
								className="cursor-pointer bg-transparent border-0 p-0 text-inherit leading-none"
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

						<TooltipContent className="p-0 bg-surface-secondary border-surface-quaternary text-content-primary">
							<OverflowY maxHeight={400}>
								<List
									component="ul"
									className="flex flex-col flex-nowrap gap-0 px-0.5 py-1"
									style={{
										fontSize: theme.typography.body2.fontSize,
									}}
								>
									{userGroups.map((group) => {
										const groupName = group.display_name || group.name;
										return (
											<ListItem
												key={group.id}
												className="gap-x-[10px] items-center"
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

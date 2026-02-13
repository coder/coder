import { useTheme } from "@emotion/react";
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
	const theme = useTheme();

	return (
		<TableCell>
			{userGroups === undefined ? (
				// Felt right to add emphasis to the undefined state for semantics
				// ("hey, this isn't normal"), but the default italics looked weird in
				// the table UI
				<em css={{ fontStyle: "normal" }}>N/A</em>
			) : (
				<TooltipProvider>
					<Tooltip delayDuration={0}>
						<TooltipTrigger asChild>
							<button
								css={{
									cursor: "pointer",
									backgroundColor: "transparent",
									border: "none",
									padding: 0,
									color: "inherit",
									lineHeight: "1",
								}}
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
									css={{
										display: "flex",
										flexFlow: "column nowrap",
										fontSize: theme.typography.body2.fontSize,
										padding: "4px 2px",
										gap: 0,
									}}
								>
									{userGroups.map((group) => {
										const groupName = group.display_name || group.name;
										return (
											<ListItem
												key={group.id}
												css={{
													columnGap: 10,
													alignItems: "center",
												}}
											>
												<Avatar
													size="sm"
													variant="icon"
													src={group.avatar_url}
													fallback={groupName}
												/>

												<span
													css={{
														whiteSpace: "nowrap",
														textOverflow: "ellipsis",
														overflow: "hidden",
														lineHeight: 1,
														margin: 0,
													}}
												>
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

import { useTheme } from "@emotion/react";
import GroupIcon from "@mui/icons-material/Group";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import type { Group } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { OverflowY } from "components/OverflowY/OverflowY";
import { TableCell } from "components/Table/Table";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { type FC, useState } from "react";

type GroupsCellProps = {
	userGroups: readonly Group[] | undefined;
};

export const UserGroupsCell: FC<GroupsCellProps> = ({ userGroups }) => {
	const theme = useTheme();
	const [open, setOpen] = useState(false);

	return (
		<TableCell>
			{userGroups === undefined ? (
				// Felt right to add emphasis to the undefined state for semantics
				// ("hey, this isn't normal"), but the default italics looked weird in
				// the table UI
				<em css={{ fontStyle: "normal" }}>N/A</em>
			) : (
				<Popover open={open} onOpenChange={setOpen}>
					<PopoverTrigger
						onMouseEnter={() => setOpen(true)}
						onMouseLeave={() => setOpen(false)}
					>
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
								<GroupIcon
									css={{
										width: "1rem",
										height: "1rem",
										opacity: userGroups.length > 0 ? 0.8 : 0.5,
									}}
								/>

								<span>
									{userGroups.length} Group{userGroups.length !== 1 && "s"}
								</span>
							</div>
						</button>
					</PopoverTrigger>

					<PopoverContent
						align="center"
						side="top"
						css={{
							minWidth: "auto",
						}}
					>
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
					</PopoverContent>
				</Popover>
			)}
		</TableCell>
	);
};

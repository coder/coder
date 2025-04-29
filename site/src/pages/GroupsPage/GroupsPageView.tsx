import type { Interpolation, Theme } from "@emotion/react";
import AddOutlined from "@mui/icons-material/AddOutlined";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import Skeleton from "@mui/material/Skeleton";
import type { Group } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { useClickableTableRow } from "hooks";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { docs } from "utils/docs";

export type GroupsPageViewProps = {
	groups: Group[] | undefined;
	canCreateGroup: boolean;
	groupsEnabled: boolean;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
	groups,
	canCreateGroup,
	groupsEnabled,
}) => {
	const isLoading = Boolean(groups === undefined);
	const isEmpty = Boolean(groups && groups.length === 0);

	return (
		<>
			<ChooseOne>
				<Cond condition={!groupsEnabled}>
					<Paywall
						message="Groups"
						description="Organize users into groups with restricted access to templates. You need a Premium license to use this feature."
						documentationLink={docs("/admin/users/groups-roles")}
					/>
				</Cond>
				<Cond>
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="w-2/5">Name</TableHead>
								<TableHead className="w-3/5">Users</TableHead>
								<TableHead className="w-auto" />
							</TableRow>
						</TableHeader>
						<TableBody>
							<ChooseOne>
								<Cond condition={isLoading}>
									<TableLoader />
								</Cond>

								<Cond condition={isEmpty}>
									<TableRow>
										<TableCell colSpan={999}>
											<EmptyState
												message="No groups yet"
												description={
													canCreateGroup
														? "Create your first group"
														: "You don't have permission to create a group"
												}
												cta={
													canCreateGroup && (
														<Button asChild>
															<RouterLink to="create">
																<AddOutlined />
																Create group
															</RouterLink>
														</Button>
													)
												}
											/>
										</TableCell>
									</TableRow>
								</Cond>

								<Cond>
									{groups?.map((group) => (
										<GroupRow key={group.id} group={group} />
									))}
								</Cond>
							</ChooseOne>
						</TableBody>
					</Table>
				</Cond>
			</ChooseOne>
		</>
	);
};

interface GroupRowProps {
	group: Group;
}

const GroupRow: FC<GroupRowProps> = ({ group }) => {
	const navigate = useNavigate();
	const rowProps = useClickableTableRow({
		onClick: () => navigate(group.name),
	});
	const memberAvatars = group.members.slice(0, 5);
	const remainingAvatars = group.members.length - memberAvatars.length;

	return (
		<TableRow data-testid={`group-${group.id}`} {...rowProps}>
			<TableCell>
				<AvatarData
					avatar={
						<Avatar
							size="lg"
							variant="icon"
							fallback={group.display_name || group.name}
							src={group.avatar_url}
						/>
					}
					title={group.display_name || group.name}
					subtitle={`${group.members.length} members`}
				/>
			</TableCell>

			<TableCell>
				{group.members.length > 0 ? (
					<div className="flex items-center gap-2">
						{memberAvatars.map((member) => (
							<Avatar
								key={member.username}
								fallback={member.username}
								src={member.avatar_url}
							/>
						))}
						{remainingAvatars > 0 && (
							<Badge className="h-[--avatar-default]">
								+{remainingAvatars}
							</Badge>
						)}
					</div>
				) : (
					"-"
				)}
			</TableCell>

			<TableCell>
				<div css={styles.arrowCell}>
					<KeyboardArrowRight css={styles.arrowRight} />
				</div>
			</TableCell>
		</TableRow>
	);
};

const TableLoader: FC = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<div css={{ display: "flex", alignItems: "center", gap: 8 }}>
						<AvatarDataSkeleton />
					</div>
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

const styles = {
	arrowRight: (theme) => ({
		color: theme.palette.text.secondary,
		width: 20,
		height: 20,
	}),
	arrowCell: {
		display: "flex",
	},
} satisfies Record<string, Interpolation<Theme>>;

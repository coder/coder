import { UserPlusIcon } from "lucide-react";
import type { ComponentProps, FC } from "react";
import { Link } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { UsersFilter } from "#/components/Filter/UsersFilter";
import {
	PaginationContainer,
	type PaginationResult,
} from "#/components/PaginationWidget/PaginationContainer";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { UsersTable, type UsersTableProps } from "./UsersTable";

type UsersPageViewProps = Omit<UsersTableProps, "users"> & {
	filterProps: ComponentProps<typeof UsersFilter>;
	usersQuery: PaginationResult<TypesGen.GetUsersResponse>;
	canCreateUser?: boolean;
};

export const UsersPageView: FC<UsersPageViewProps> = ({
	filterProps,
	usersQuery,
	canCreateUser,
	...props
}) => {
	return (
		<>
			<SettingsHeader
				actions={
					canCreateUser && (
						<Button asChild>
							<Link to="create">
								<UserPlusIcon />
								Create user
							</Link>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>Users</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Manage user accounts and permissions.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<UsersFilter {...filterProps} />

			<PaginationContainer query={usersQuery} paginationUnitLabel="users">
				<UsersTable users={usersQuery.data?.users} {...props} />
			</PaginationContainer>
		</>
	);
};

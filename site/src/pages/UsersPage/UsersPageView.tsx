import { UserPlusIcon } from "lucide-react";
import { type ComponentProps, type FC, useState } from "react";
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
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	UserActionDialogs,
	type UserAdminAction,
} from "#/modules/users/UserActionDialogs";
import { docs } from "#/utils/docs";
import { UsersTable, type UsersTableProps } from "./UsersTable";

type UsersPageViewProps = Omit<UsersTableProps, "users" | "onAction"> & {
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
	const [action, setAction] = useState<UserAdminAction>();

	return (
		<>
			<SettingsHeader
				actions={
					canCreateUser && (
						<Button asChild>
							<Link to="create">
								<UserPlusIcon />
								New user
							</Link>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>Users</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Manage user accounts and permissions.{" "}
					<SettingsHeaderDocsLink href={docs("/admin/users")} />
				</SettingsHeaderDescription>
			</SettingsHeader>

			<UsersFilter {...filterProps} />

			<PaginationContainer query={usersQuery} paginationUnitLabel="users">
				<UsersTable
					{...props}
					users={usersQuery.data?.users}
					onAction={setAction}
				/>
			</PaginationContainer>

			<UserActionDialogs action={action} onClose={() => setAction(undefined)} />
		</>
	);
};

import { API } from "api/api";
import { Avatar } from "components/Avatar/Avatar";
import {
	SelectFilter,
	type SelectFilterOption,
	SelectFilterSearch,
} from "components/Filter/SelectFilter";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { FC } from "react";
import { type UseFilterMenuOptions, useFilterMenu } from "./menu";

export const useUserFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	const { user: me } = useAuthenticated();

	const addMeAsFirstOption = (options: readonly SelectFilterOption[]) => {
		const filtered = options.filter((o) => o.value !== me.username);
		return [
			{
				label: me.username,
				value: me.username,
				startIcon: (
					<Avatar fallback={me.username} src={me.avatar_url} size="sm" />
				),
			},
			...filtered,
		];
	};

	return useFilterMenu({
		onChange,
		enabled,
		value,
		id: "owner",
		getSelectedOption: async () => {
			if (value === "me") {
				return {
					label: me.username,
					value: me.username,
					startIcon: (
						<Avatar fallback={me.username} src={me.avatar_url} size="sm" />
					),
				};
			}

			const usersRes = await API.getUsers({ q: value, limit: 1 });
			const firstUser = usersRes.users.at(0);
			if (firstUser && firstUser.username === value) {
				return {
					label: firstUser.username,
					value: firstUser.username,
					startIcon: (
						<Avatar
							fallback={firstUser.username}
							src={firstUser.avatar_url}
							size="sm"
						/>
					),
				};
			}
			return null;
		},
		getOptions: async (query) => {
			const usersRes = await API.getUsers({ q: query, limit: 25 });
			let options = usersRes.users.map<SelectFilterOption>((user) => ({
				label: user.username,
				value: user.username,
				startIcon: (
					<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
				),
			}));
			options = addMeAsFirstOption(options);
			return options;
		},
	});
};

export type UserFilterMenu = ReturnType<typeof useUserFilterMenu>;

interface UserMenuProps {
	menu: UserFilterMenu;
	width?: number;
}

export const UserMenu: FC<UserMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Select user"
			placeholder="All users"
			emptyText="No users found"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			selectFilterSearch={
				<SelectFilterSearch
					inputProps={{ "aria-label": "Search user" }}
					placeholder="Search user..."
					value={menu.query}
					onChange={menu.setQuery}
				/>
			}
			width={width}
		/>
	);
};

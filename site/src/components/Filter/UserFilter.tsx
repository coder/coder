import { API } from "api/api";
import {
	SelectFilter,
	type SelectFilterOption,
	SelectFilterSearch,
} from "components/Filter/SelectFilter";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
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
					<UserAvatar
						username={me.username}
						avatarURL={me.avatar_url}
						size="xs"
					/>
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
						<UserAvatar
							username={me.username}
							avatarURL={me.avatar_url}
							size="xs"
						/>
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
						<UserAvatar
							username={firstUser.username}
							avatarURL={firstUser.avatar_url}
							size="xs"
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
					<UserAvatar
						username={user.username}
						avatarURL={user.avatar_url}
						size="xs"
					/>
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

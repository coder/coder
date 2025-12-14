import type { Group, ReducedUser, User } from "api/typesGenerated";
import { AvatarData } from "components/Avatar/AvatarData";
import type { HTMLAttributes } from "react";
import { getGroupSubtitle } from "utils/groups";

type UserOrGroupAutocompleteValue = User | ReducedUser | Group | null;

type UserOption = User | ReducedUser;
type OptionType = UserOption | Group;

/**
 * Type guard to check if the value is a Group.
 * Groups have a "members" property that users don't have.
 */
export const isGroup = (
	value: UserOrGroupAutocompleteValue,
): value is Group => {
	return value !== null && typeof value === "object" && "members" in value;
};

interface UserOrGroupOptionProps {
	option: OptionType;
	htmlProps: HTMLAttributes<HTMLLIElement>;
}

export const UserOrGroupOption = ({
	option,
	htmlProps,
}: UserOrGroupOptionProps) => {
	const isOptionGroup = isGroup(option);

	return (
		<li {...htmlProps}>
			<AvatarData
				title={
					isOptionGroup ? option.display_name || option.name : option.username
				}
				subtitle={isOptionGroup ? getGroupSubtitle(option) : option.email}
				src={option.avatar_url}
			/>
		</li>
	);
};

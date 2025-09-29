import Skeleton from "@mui/material/Skeleton";
import { users } from "api/queries/users";
import type { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { useAuthenticated } from "hooks";
import { useDebouncedValue } from "hooks/debounce";
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { cn } from "utils/cn";

type UserOption = {
	label: string;
	/**
	 * The username of the user.
	 */
	value: string;
	avatarUrl?: string;
};

type UsersComboboxProps = {
	value: string;
	onValueChange: (value: string) => void;
};

export const UsersCombobox: FC<UsersComboboxProps> = ({
	value,
	onValueChange,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search, 250);
	const { user } = useAuthenticated();
	const { data: options } = useQuery({
		...users({ q: debouncedSearch }),
		select: (res) => mapUsersToOptions(res.users, user, value),
		placeholderData: keepPreviousData,
	});
	const selectedOption = options?.find((o) => o.value === value);

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					disabled={!options}
					variant="outline"
					role="combobox"
					aria-expanded={open}
					className="w-[280px] justify-between"
				>
					{options ? (
						selectedOption ? (
							<UserItem option={selectedOption} className="-ml-1" />
						) : (
							"Select user..."
						)
					) : (
						<Skeleton variant="text" className="w-[120px] h-3" />
					)}
					<ChevronsUpDownIcon className="ml-2 h-4 w-4 shrink-0 opacity-50" />
				</Button>
			</PopoverTrigger>
			<PopoverContent className="w-[280px] p-0">
				<Command>
					<CommandInput
						placeholder="Search user..."
						value={search}
						onValueChange={setSearch}
					/>
					<CommandList>
						<CommandEmpty>No users found.</CommandEmpty>
						<CommandGroup>
							{options?.map((option) => (
								<CommandItem
									key={option.value}
									value={option.value}
									onSelect={() => {
										onValueChange(option.value);
										setOpen(false);
									}}
								>
									<UserItem option={option} />
									<CheckIcon
										className={cn(
											"ml-2 h-4 w-4",
											option.value === selectedOption?.value
												? "opacity-100"
												: "opacity-0",
										)}
									/>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

type UserItemProps = {
	option: UserOption;
	className?: string;
};

const UserItem: FC<UserItemProps> = ({ option, className }) => {
	return (
		<div className={cn("flex flex-1 items-center gap-2", className)}>
			<Avatar src={option.avatarUrl} fallback={option.label} />
			{option.label}
		</div>
	);
};

function mapUsersToOptions(
	users: readonly User[],
	/**
	 * Includes the authenticated user in the list if they are not already
	 * present. So the current user can always select themselves easily.
	 */
	authUser: User,
	/**
	 * Username of the currently selected user.
	 */
	selectedValue: string,
): UserOption[] {
	const includeAuthenticatedUser = (users: readonly User[]) => {
		const hasAuthenticatedUser = users.some(
			(u) => u.username === authUser.username,
		);
		if (hasAuthenticatedUser) {
			return users;
		}
		return [authUser, ...users];
	};

	const sortSelectedFirst = (a: User) =>
		selectedValue && a.username === selectedValue ? -1 : 0;

	return includeAuthenticatedUser(users)
		.toSorted(sortSelectedFirst)
		.map((user) => ({
			label: user.name || user.username,
			value: user.username,
			avatarUrl: user.avatar_url,
		}));
}

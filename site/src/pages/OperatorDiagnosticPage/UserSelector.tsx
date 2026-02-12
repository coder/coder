import { Button } from "components/Button/Button";
import type { FC } from "react";
import { MOCK_USERNAMES } from "./mockDataService";

interface UserSelectorProps {
	currentUsername: string;
	onSelect: (username: string) => void;
}

export const UserSelector: FC<UserSelectorProps> = ({
	currentUsername,
	onSelect,
}) => {
	return (
		<div className="flex items-center gap-2 flex-wrap">
			{MOCK_USERNAMES.map((name) => (
				<Button
					key={name}
					size="sm"
					variant={name === currentUsername ? "default" : "outline"}
					onClick={() => onSelect(name)}
				>
					{name}
				</Button>
			))}
		</div>
	);
};

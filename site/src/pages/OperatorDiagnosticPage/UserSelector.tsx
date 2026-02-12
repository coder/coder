import { Button } from "components/Button/Button";
import type { FC } from "react";
import { useRef } from "react";
import { Link } from "react-router";
import { MOCK_USERNAMES } from "./mockDataService";

interface UserSelectorProps {
	currentUsername: string;
	onSelect: (username: string) => void;
	isDemo: boolean;
}

export const UserSelector: FC<UserSelectorProps> = ({
	currentUsername,
	onSelect,
	isDemo,
}) => {
	const inputRef = useRef<HTMLInputElement>(null);

	if (isDemo) {
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
				<Link to={`/connectionlog/diagnostics/${currentUsername}`}>
					<Button size="sm" variant="subtle">
						Live
					</Button>
				</Link>
			</div>
		);
	}

	return (
		<div className="flex items-center gap-2">
			<form
				className="flex items-center gap-2"
				onSubmit={(e) => {
					e.preventDefault();
					const value = inputRef.current?.value.trim();
					if (value) onSelect(value);
				}}
			>
				<input
					ref={inputRef}
					defaultValue={currentUsername}
					placeholder="Enter username..."
					className="h-9 rounded-md border border-border bg-transparent px-3 text-sm text-content-primary placeholder:text-content-secondary focus:outline-none focus:ring-2 focus:ring-content-primary"
				/>
				<Button type="submit" size="sm">
					Go
				</Button>
			</form>
			<Link to={`/connectionlog/diagnostics/${currentUsername}?demo=true`}>
				<Button size="sm" variant="outline">
					Demo
				</Button>
			</Link>
		</div>
	);
};

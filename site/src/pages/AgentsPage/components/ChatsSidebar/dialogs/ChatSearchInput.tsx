import { SearchIcon } from "lucide-react";
import type {
	ChangeEventHandler,
	FC,
	KeyboardEventHandler,
	RefObject,
} from "react";
import { Input } from "#/components/Input/Input";

type ChatSearchInputProps = {
	readonly activeResultId: string | undefined;
	readonly hasResults: boolean;
	readonly inputRef: RefObject<HTMLInputElement | null>;
	readonly listboxId: string;
	readonly value: string;
	readonly onChange: ChangeEventHandler<HTMLInputElement>;
	readonly onKeyDown: KeyboardEventHandler<HTMLInputElement>;
};

export const ChatSearchInput: FC<ChatSearchInputProps> = ({
	activeResultId,
	hasResults,
	inputRef,
	listboxId,
	value,
	onChange,
	onKeyDown,
}) => {
	return (
		<div className="relative min-w-0">
			<SearchIcon className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-content-secondary" />
			<Input
				ref={inputRef}
				value={value}
				onChange={onChange}
				onKeyDown={onKeyDown}
				placeholder="Search chats..."
				className="h-10 border-border-default bg-surface-primary pl-9 pr-3 placeholder:text-content-disabled"
				aria-label="Search chats"
				role="combobox"
				aria-controls={hasResults ? listboxId : undefined}
				aria-expanded={hasResults}
				aria-haspopup="listbox"
				aria-activedescendant={activeResultId}
			/>
		</div>
	);
};

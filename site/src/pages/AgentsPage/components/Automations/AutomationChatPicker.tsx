import { useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { chat, chatSearch } from "#/api/queries/chats";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";

type AutomationChatPickerProps = {
	id: string;
	value: string;
	currentUserId: string;
	invalid: boolean;
	describedBy?: string;
	onChange: (chatId: string) => void;
};

/** Picks one of the current user's root chats as an automation target. */
export const AutomationChatPicker: React.FC<AutomationChatPickerProps> = ({
	id,
	value,
	currentUserId,
	invalid,
	describedBy,
	onChange,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	// Quotes make the text one backend token; the backend has no escaping.
	const debouncedSearch = useDebouncedValue(
		search.replaceAll('"', "").trim(),
		300,
	);
	// The picker shows titles only, so match a title substring. The full-text
	// `search:` term matches whole words and message content instead.
	const searchQuery = useQuery({
		...chatSearch({
			q: debouncedSearch
				? `title:"${debouncedSearch}" archived:false`
				: "archived:false",
		}),
		enabled: open,
		placeholderData: keepPreviousData,
	});
	// The chats API returns root chats only; automations target your own.
	const chats = (searchQuery.data ?? []).filter(
		(c) => c.owner_id === currentUserId,
	);
	const listedChat = chats.find((c) => c.id === value);
	const selectedQuery = useQuery({
		...chat(value),
		enabled: Boolean(value) && !listedChat,
	});
	const selectedChat = listedChat ?? selectedQuery.data;
	let triggerLabel = "Select a chat";
	if (value) {
		triggerLabel = selectedQuery.isLoading
			? "Loading chat"
			: selectedChat?.title || "Untitled";
	}

	return (
		<Combobox
			value={value}
			onValueChange={(chatId) => {
				if (chatId) {
					onChange(chatId);
				}
			}}
			open={open}
			onOpenChange={(nextOpen) => {
				setOpen(nextOpen);
				if (!nextOpen) {
					setSearch("");
				}
			}}
		>
			<ComboboxTrigger asChild>
				<Button
					id={id}
					variant="outline"
					className="justify-between"
					aria-invalid={invalid}
					aria-describedby={describedBy}
				>
					<span className="truncate">{triggerLabel}</span>
					<ChevronDownIcon className="p-0.5" />
				</Button>
			</ComboboxTrigger>
			<ComboboxContent
				shouldFilter={false}
				className="w-(--radix-popover-trigger-width)"
			>
				<ComboboxInput
					placeholder="Search chats"
					value={search}
					onValueChange={setSearch}
				/>
				<ComboboxList>
					{searchQuery.isFetching && (
						<div className="flex justify-center p-2">
							<Spinner loading />
						</div>
					)}
					{searchQuery.isError && (
						<p className="m-0 p-2 text-sm text-content-destructive">
							Could not load chats.
						</p>
					)}
					{!searchQuery.isLoading && (
						<ComboboxEmpty>No chats found.</ComboboxEmpty>
					)}
					{chats.map((c) => (
						<ComboboxItem key={c.id} value={c.id}>
							<span className="flex-1 truncate">{c.title || "Untitled"}</span>
						</ComboboxItem>
					))}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>
	);
};

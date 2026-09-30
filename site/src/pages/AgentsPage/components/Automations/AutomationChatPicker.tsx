import { useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorStatus } from "#/api/errors";
import { chat, chatSearch } from "#/api/queries/chats";
import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";

type AutomationChatPickerProps = Pick<
	React.ComponentProps<"button">,
	"id" | "aria-invalid" | "aria-describedby"
> & {
	value: string;
	currentUserId: string;
	onChange: (chatId: string) => void;
};

/** Picks one of the current user's root chats as an automation target. */
export const AutomationChatPicker: React.FC<AutomationChatPickerProps> = ({
	value,
	currentUserId,
	onChange,
	...buttonProps
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
	const chats = (searchQuery.data ?? []).filter(
		(c) => c.owner_id === currentUserId,
	);
	const listedChat = chats.find((c) => c.id === value);
	const selectedQuery = useQuery({
		...chat(value),
		enabled: Boolean(value) && !listedChat,
	});
	let selectedLabel: string | undefined;
	if (listedChat) {
		selectedLabel = listedChat.title || "Untitled";
	} else if (selectedQuery.data) {
		selectedLabel = selectedQuery.data.title || "Untitled";
	} else if (selectedQuery.isLoading) {
		selectedLabel = "Loading chat";
	} else if (value) {
		selectedLabel =
			getErrorStatus(selectedQuery.error) === 404
				? "Chat not found"
				: "Could not load chat";
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
				<ComboboxButton
					{...buttonProps}
					selectedOption={
						selectedLabel ? { label: selectedLabel, value } : undefined
					}
					placeholder="Select a chat"
				/>
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
					{!searchQuery.isLoading && !searchQuery.isError && (
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

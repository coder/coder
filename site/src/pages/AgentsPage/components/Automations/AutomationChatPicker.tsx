import { useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorStatus } from "#/api/errors";
import { chat, chatSearch } from "#/api/queries/chats";
import type { Chat } from "#/api/typesGenerated";
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
import { CommandGroup } from "#/components/Command/Command";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";

type AutomationChatPickerProps = Pick<
	React.ComponentProps<"button">,
	"id" | "aria-invalid" | "aria-describedby"
> & {
	organizationId: string;
	value: string;
	onChange: (chatId: string) => void;
};

/** True for a chat that a new-chat automation created for one run. */
export const isAutomationRunChat = (chat: Pick<Chat, "automation_id">) =>
	Boolean(chat.automation_id);

/** Picks one of the current user's root chats as an automation target. */
export const AutomationChatPicker: React.FC<AutomationChatPickerProps> = ({
	organizationId,
	value,
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
	const searchTerms = [
		"archived:false",
		"source:created_by_me",
		`organization:${organizationId}`,
	];
	if (debouncedSearch) {
		searchTerms.unshift(`title:"${debouncedSearch}"`);
	}
	const searchQuery = useQuery({
		...chatSearch({ q: searchTerms.join(" ") }),
		enabled: open,
		placeholderData: keepPreviousData,
	});
	const chats = searchQuery.data ?? [];
	// Run chats of new-chat automations list after the user's own chats, so
	// a busy schedule cannot push them out of view. Search finds both.
	const ownChats = chats.filter((c) => !isAutomationRunChat(c));
	const runChats = chats.filter((c) => isAutomationRunChat(c));
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
				// A fixed height, not the available height: shrinking to the space
				// left below the trigger stops Radix from flipping the list above
				// it, so it opened under the dialog footer.
				className="max-h-72 w-(--radix-popover-trigger-width)"
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
					{!searchQuery.isFetching && !searchQuery.isError && (
						<ComboboxEmpty>No chats found.</ComboboxEmpty>
					)}
					{ownChats.map((c) => (
						<ComboboxItem key={c.id} value={c.id}>
							<span className="flex-1 truncate">{c.title || "Untitled"}</span>
						</ComboboxItem>
					))}
					{runChats.length > 0 && (
						<CommandGroup heading="Automation runs" className="p-0">
							{runChats.map((c) => (
								<ComboboxItem key={c.id} value={c.id}>
									<span className="flex-1 truncate">
										{c.title || "Untitled"}
									</span>
								</ComboboxItem>
							))}
						</CommandGroup>
					)}
				</ComboboxList>
			</ComboboxContent>
		</Combobox>
	);
};

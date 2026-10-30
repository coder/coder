import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";
import { useRef, useState } from "react";
import type { ChatProjectTriggerSource } from "#/api/typesGenerated";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { SettingsCardAddRow } from "./ProjectSection";
import { TriggerSourceIcon } from "./TriggerSourceIcon";
import { triggerSources } from "./triggerCatalog";

/** A source with events to pick from. */
type EventTriggerSource = Exclude<
	ChatProjectTriggerSource,
	"webhook" | "schedule"
>;

const eventTriggerSources: readonly EventTriggerSource[] = ["github", "slack"];

type TriggerAddMenuProps = {
	/** The "Add trigger" row, for moving focus to it. */
	readonly ref?: React.Ref<HTMLButtonElement>;
	/** True while a trigger is being created. The row ignores input. */
	readonly isCreating: boolean;
	readonly onCreate: (
		source: ChatProjectTriggerSource,
		events: readonly string[],
	) => void;
	/** Opens the form a schedule trigger needs before it can be created. */
	readonly onAddSchedule: () => void;
};

/**
 * The "Add trigger" row of the triggers card. It opens a searchable menu of
 * sources; GitHub and Slack open a page of their events.
 */
export const TriggerAddMenu: React.FC<TriggerAddMenuProps> = ({
	ref,
	isCreating,
	onCreate,
	onAddSchedule,
}) => {
	const [open, setOpen] = useState(false);
	const [page, setPage] = useState<EventTriggerSource>();
	const [search, setSearch] = useState("");
	const searchRef = useRef<HTMLInputElement>(null);

	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		if (!next) {
			setPage(undefined);
			setSearch("");
		}
	};
	// Clicking an item moves focus to the list, so it goes back to the
	// search box to keep typing and Backspace working on the new page.
	const openPage = (source: EventTriggerSource | undefined) => {
		setPage(source);
		setSearch("");
		searchRef.current?.focus();
	};
	const choose = (
		source: ChatProjectTriggerSource,
		events: readonly string[],
	) => {
		handleOpenChange(false);
		onCreate(source, events);
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<SettingsCardAddRow
					ref={ref}
					label="Add trigger"
					aria-disabled={isCreating}
					onClick={(event) => {
						// Keeps the menu closed while a trigger is being created.
						if (isCreating) {
							event.preventDefault();
						}
					}}
				/>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-72 p-0">
				<Command>
					<CommandInput
						ref={searchRef}
						aria-label={
							page
								? `Search ${triggerSources[page].title} events`
								: "Search triggers"
						}
						placeholder={
							page
								? `Search ${triggerSources[page].title} events...`
								: "Search triggers..."
						}
						value={search}
						onValueChange={setSearch}
						onKeyDown={(event) => {
							if (event.key === "Backspace" && search === "" && page) {
								setPage(undefined);
							}
						}}
					/>
					<CommandList>
						<CommandEmpty>No triggers found.</CommandEmpty>
						{page ? (
							<CommandGroup>
								<CommandItem value="Back" onSelect={() => openPage(undefined)}>
									<ChevronLeftIcon aria-hidden />
									Back
								</CommandItem>
								<EventItems source={page} onChoose={choose} />
							</CommandGroup>
						) : (
							<>
								<CommandGroup>
									<SourceItems
										onOpenPage={openPage}
										onChoose={choose}
										onAddSchedule={() => {
											handleOpenChange(false);
											onAddSchedule();
										}}
									/>
								</CommandGroup>
								{/* Searching from the top level also finds events. */}
								{search !== "" &&
									eventTriggerSources.map((source) => (
										<CommandGroup
											key={source}
											heading={triggerSources[source].title}
										>
											<EventItems source={source} onChoose={choose} />
										</CommandGroup>
									))}
							</>
						)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};

type SourceItemsProps = {
	readonly onOpenPage: (source: EventTriggerSource) => void;
	readonly onChoose: TriggerAddMenuProps["onCreate"];
	readonly onAddSchedule: () => void;
};

/** GitHub and Slack, which open a page of their events, then Webhook and Schedule. */
const SourceItems: React.FC<SourceItemsProps> = ({
	onOpenPage,
	onChoose,
	onAddSchedule,
}) => (
	<>
		{eventTriggerSources.map((source) => (
			<CommandItem
				key={source}
				value={triggerSources[source].title}
				onSelect={() => onOpenPage(source)}
			>
				<TriggerSourceIcon source={source} />
				<span className="flex-1">{triggerSources[source].title}</span>
				<ChevronRightIcon aria-hidden />
			</CommandItem>
		))}
		<CommandItem
			value={triggerSources.webhook.title}
			onSelect={() => onChoose("webhook", [])}
		>
			<TriggerSourceIcon source="webhook" />
			{triggerSources.webhook.title}
		</CommandItem>
		<CommandItem value={triggerSources.schedule.title} onSelect={onAddSchedule}>
			<TriggerSourceIcon source="schedule" />
			{triggerSources.schedule.title}
		</CommandItem>
	</>
);

type EventItemsProps = {
	readonly source: EventTriggerSource;
	readonly onChoose: TriggerAddMenuProps["onCreate"];
};

const EventItems: React.FC<EventItemsProps> = ({ source, onChoose }) => {
	const { title, events, anyEventLabel } = triggerSources[source];
	return (
		<>
			{events.map((event) => (
				<CommandItem
					key={event.value}
					value={`${title} ${event.optionLabel}`}
					onSelect={() => onChoose(source, [event.value])}
				>
					{event.optionLabel}
				</CommandItem>
			))}
			{source === "github" && (
				<CommandItem
					value={`${title} ${anyEventLabel}`}
					onSelect={() => onChoose(source, [])}
				>
					{anyEventLabel}
				</CommandItem>
			)}
		</>
	);
};

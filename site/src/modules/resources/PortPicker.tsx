import { Command as CommandPrimitive } from "cmdk";
import {
	ExternalLinkIcon,
	PlusIcon,
	RadioIcon,
	SearchIcon,
} from "lucide-react";
import { DismissableLayer } from "radix-ui/internal";
import { type FC, useState } from "react";
import type { WorkspaceAgentListeningPort } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { MAX_PORT, MIN_PORT } from "#/utils/portForward";

const parsePort = (value: string): number | undefined => {
	const port = Number.parseInt(value, 10);
	return Number.isInteger(port) && port >= MIN_PORT && port <= MAX_PORT
		? port
		: undefined;
};

const matchesQuery = (
	port: WorkspaceAgentListeningPort,
	query: string,
): boolean =>
	port.port.toString().includes(query) ||
	port.process_name.toLowerCase().includes(query.toLowerCase());

interface ListeningPortOptionProps {
	port: WorkspaceAgentListeningPort;
	onSelect: (port: string) => void;
}

const ListeningPortOption: FC<ListeningPortOptionProps> = ({
	port,
	onSelect,
}) => (
	<CommandItem
		value={port.port.toString()}
		className="px-3 whitespace-nowrap"
		onSelect={onSelect}
	>
		<RadioIcon />
		<span>{port.port}</span>
		<span className="ml-auto text-content-secondary">
			{port.process_name || "Unknown process"}
		</span>
	</CommandItem>
);

interface PortPickerProps {
	listeningPorts: readonly WorkspaceAgentListeningPort[];
	onConnect: (port: number) => void;
}

export const PortPicker: FC<PortPickerProps> = ({
	listeningPorts,
	onConnect,
}) => {
	const [query, setQuery] = useState("");
	const [menuOpen, setMenuOpen] = useState(false);

	// Ports whose number starts with the query rank above the typed port, which
	// in turn ranks above looser matches on the port number or process name.
	const prefixMatches = listeningPorts.filter((port) =>
		port.port.toString().startsWith(query),
	);
	const looseMatches = listeningPorts.filter(
		(port) =>
			!port.port.toString().startsWith(query) && matchesQuery(port, query),
	);
	const selectedPort = parsePort(query);
	const typedPort = listeningPorts.some((port) => port.port === selectedPort)
		? undefined
		: selectedPort;
	const hasOptions =
		prefixMatches.length > 0 ||
		looseMatches.length > 0 ||
		typedPort !== undefined;

	const connect = () => {
		if (selectedPort !== undefined) {
			onConnect(selectedPort);
		}
	};

	const pick = (port: string) => {
		setQuery(port);
		setMenuOpen(false);
	};

	return (
		<>
			<Command
				label="Connect to port"
				shouldFilter={false}
				className="relative mt-2 flex-1 overflow-visible bg-transparent"
			>
				<div className="flex h-[34px] items-center gap-2 rounded-md border border-solid border-border px-2.5 focus-within:border-content-link">
					<SearchIcon className="size-icon-xs shrink-0 text-content-secondary" />
					{/* `CommandInput` is the full-width search row of a command palette,
					    which cannot be styled down to a single form field. */}
					<CommandPrimitive.Input
						className="h-full w-full border-none bg-transparent text-sm text-content-primary outline-hidden placeholder:text-content-secondary"
						inputMode="numeric"
						placeholder="Connect to port..."
						value={query}
						onValueChange={(value) => {
							setQuery(value);
							setMenuOpen(true);
						}}
						onClick={() => setMenuOpen(true)}
						onBlur={() => setMenuOpen(false)}
						onKeyDown={(event) => {
							if (event.key === "ArrowDown") {
								setMenuOpen(true);
								return;
							}
							// cmdk handles Enter while the menu is open by picking the
							// active option.
							if (event.key === "Enter" && !menuOpen) {
								connect();
							}
						}}
					/>
				</div>
				{menuOpen && (
					// The menu is its own dismissable layer so Escape and outside clicks
					// close it without also closing the surrounding popover.
					<DismissableLayer.Root asChild onDismiss={() => setMenuOpen(false)}>
						<CommandList
							// Keeping focus in the field means selecting an option does not
							// close the menu through the input's blur handler.
							onMouseDown={(event) => event.preventDefault()}
							className="absolute top-full left-0 z-20 mt-1 max-h-60 w-full min-w-56 rounded-md border border-solid border-border bg-surface-primary p-2 shadow-md"
						>
							{prefixMatches.map((port) => (
								<ListeningPortOption
									key={port.port}
									port={port}
									onSelect={pick}
								/>
							))}
							{typedPort !== undefined && (
								<CommandItem
									value={typedPort.toString()}
									className="px-3 whitespace-nowrap"
									onSelect={pick}
								>
									<PlusIcon />
									<span>Use port {typedPort}</span>
								</CommandItem>
							)}
							{looseMatches.map((port) => (
								<ListeningPortOption
									key={port.port}
									port={port}
									onSelect={pick}
								/>
							))}
							{!hasOptions && (
								<div className="px-3 py-4 text-center text-content-secondary">
									{query
										? `Enter a port from ${MIN_PORT} to ${MAX_PORT}.`
										: "Enter a port number to connect."}
								</div>
							)}
						</CommandList>
					</DismissableLayer.Root>
				)}
			</Command>
			<Button
				className="mt-2"
				size="sm"
				variant="outline"
				disabled={selectedPort === undefined}
				onClick={connect}
			>
				<ExternalLinkIcon />
				Connect
			</Button>
		</>
	);
};

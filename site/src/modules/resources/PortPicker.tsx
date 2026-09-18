import { ExternalLinkIcon, PlusIcon, RadioIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { WorkspaceAgentListeningPort } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
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
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { MAX_PORT, MIN_PORT } from "#/utils/portForward";

const parsePort = (value: string): number | undefined => {
	const port = Number.parseInt(value, 10);
	return Number.isInteger(port) && port >= MIN_PORT && port <= MAX_PORT
		? port
		: undefined;
};

interface ListeningPortOptionProps {
	port: WorkspaceAgentListeningPort;
}

const ListeningPortOption: FC<ListeningPortOptionProps> = ({ port }) => (
	<ComboboxItem value={port.port.toString()} className="px-3">
		<RadioIcon />
		<span>{port.port}</span>
		<span className="ml-auto text-content-secondary">
			{port.process_name || "Unknown process"}
		</span>
	</ComboboxItem>
);

interface PortPickerProps {
	listeningPorts: readonly WorkspaceAgentListeningPort[];
	onConnect: (port: number) => void;
}

export const PortPicker: FC<PortPickerProps> = ({
	listeningPorts,
	onConnect,
}) => {
	const [selectedPort, setSelectedPort] = useState("");
	const [query, setQuery] = useState("");
	const [menuOpen, setMenuOpen] = useState(false);
	const connectButtonRef = useRef<HTMLButtonElement>(null);
	// Picking a port moves focus to the connect button so Enter opens it.
	// Dismissing the menu keeps the default focus return to the trigger.
	const focusConnectOnClose = useRef(false);

	const prefixMatches = listeningPorts.filter((port) =>
		port.port.toString().startsWith(query),
	);
	const substringMatches = listeningPorts.filter((port) => {
		const portText = port.port.toString();
		return portText.includes(query) && !portText.startsWith(query);
	});
	const typedPort = parsePort(query);
	const customPort = listeningPorts.some((port) => port.port === typedPort)
		? undefined
		: typedPort;

	return (
		<Combobox
			value={selectedPort}
			onValueChange={(value) => {
				focusConnectOnClose.current = true;
				// Reselecting the active option reports undefined; keep it selected.
				if (value) {
					setSelectedPort(value);
				}
			}}
			open={menuOpen}
			onOpenChange={(open) => {
				setMenuOpen(open);
				if (!open) {
					setQuery("");
				}
			}}
		>
			<div className="mt-2 flex w-full items-center rounded border border-solid border-border focus-within:border-content-link">
				<ComboboxTrigger asChild>
					<ComboboxButton
						aria-label={
							selectedPort
								? `Connect to port ${selectedPort}`
								: "Connect to port..."
						}
						variant="subtle"
						size="sm"
						selectedOption={
							selectedPort
								? { label: selectedPort, value: selectedPort }
								: undefined
						}
						placeholder="Connect to port..."
					/>
				</ComboboxTrigger>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							ref={connectButtonRef}
							size="icon"
							variant="subtle"
							onClick={() => {
								const port = parsePort(selectedPort);
								if (port !== undefined) {
									onConnect(port);
								} else {
									setMenuOpen(true);
								}
							}}
						>
							<ExternalLinkIcon />
							<span className="sr-only">Connect to selected port</span>
						</Button>
					</TooltipTrigger>
					<TooltipContent disablePortal>
						Connect to selected port
					</TooltipContent>
				</Tooltip>
			</div>
			<ComboboxContent
				aria-label="Port picker"
				align="start"
				className="w-(--radix-popover-trigger-width) p-0"
				commandLabel="Filter or enter port"
				onCloseAutoFocus={(event) => {
					if (focusConnectOnClose.current) {
						event.preventDefault();
						connectButtonRef.current?.focus();
						focusConnectOnClose.current = false;
					}
				}}
				shouldFilter={false}
			>
				<ComboboxInput
					inputMode="numeric"
					pattern="[0-9]*"
					placeholder="Filter or enter port..."
					value={query}
					onValueChange={setQuery}
				/>
				<ComboboxList>
					{prefixMatches.map((port) => (
						<ListeningPortOption key={port.port} port={port} />
					))}
					{customPort !== undefined && (
						<ComboboxItem value={customPort.toString()} className="px-3">
							<PlusIcon />
							<span>Use port {customPort}</span>
						</ComboboxItem>
					)}
					{substringMatches.map((port) => (
						<ListeningPortOption key={port.port} port={port} />
					))}
				</ComboboxList>
				<ComboboxEmpty>
					{query
						? `Enter a port from ${MIN_PORT} to ${MAX_PORT}.`
						: "Enter a port number to connect."}
				</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};

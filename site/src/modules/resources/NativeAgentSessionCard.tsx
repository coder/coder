import type { InitializeResponse, SessionInfo } from "@agentclientprotocol/sdk";
import { cn } from "cn";
import { BotIcon, TerminalIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	NativeAgentSessionStatus,
	NativeAgentTimestamp,
} from "./NativeAgentSessionStatus";
import {
	type NativeAgentHarness,
	nativeAgentIdentity,
	nativeAgentMetadata,
} from "./nativeAgentMetadata";

// ACP identifies the harness, not its provider. Only known identities use local logos.
const nativeAgentIcons = new Map([
	["claude-acp-bridge", "/icon/claude-code.svg"],
	["claude-code", "/icon/claude-code.svg"],
	["claude-code-acp", "/icon/claude-code.svg"],
	["codex", "/icon/openai-codex.svg"],
	["codex-acp", "/icon/openai-codex.svg"],
	["gemini", "/icon/gemini.svg"],
	["gemini-cli", "/icon/gemini.svg"],
	["opencode", "/icon/opencode.svg"],
]);

type NativeAgentSessionCardProps = {
	info: SessionInfo;
	agentInfo: InitializeResponse["agentInfo"];
	harnesses: readonly NativeAgentHarness[] | undefined;
	selected: boolean;
	expanded: boolean;
	disabled: boolean;
	openDisabled: boolean;
	transcriptID?: string;
	now?: number;
	onSelect: (session: SessionInfo) => void;
	onOpen: (session: SessionInfo) => void;
};

/** Selects a native session while keeping its full identity accessible when truncated. */
export const NativeAgentSessionCard: React.FC<NativeAgentSessionCardProps> = ({
	info,
	agentInfo,
	harnesses,
	selected,
	expanded,
	disabled,
	openDisabled,
	transcriptID,
	now,
	onSelect,
	onOpen,
}) => {
	const identity = nativeAgentIdentity(info, agentInfo, harnesses);
	const agentLabel = identity.title;
	const icon = nativeAgentIcons.get(identity.name);
	const metadata = nativeAgentMetadata(info);
	return (
		<div
			className={cn(
				"flex w-full min-w-0 flex-col rounded-md border border-solid border-border sm:w-72",
				selected && "border-content-link bg-surface-secondary",
			)}
		>
			<Tooltip>
				<TooltipTrigger asChild>
					<Button
						variant="outline"
						className="h-auto w-full min-w-0 justify-start gap-3 border-0 p-3 text-left"
						aria-pressed={selected}
						aria-expanded={expanded}
						aria-controls={transcriptID}
						disabled={disabled}
						onClick={() => onSelect(info)}
					>
						{icon ? <ExternalImage src={icon} alt="" /> : <BotIcon />}
						<span className="min-w-0 space-y-1">
							<span className="block truncate font-medium">
								{info.title || agentLabel}
							</span>
							<span className="block truncate font-mono text-xs text-content-secondary">
								{info.sessionId}
							</span>
							<span className="block text-xs font-normal text-content-secondary">
								<NativeAgentTimestamp
									timestamp={metadata.startedAt}
									prefix="Started"
									now={now}
								/>
							</span>
							<NativeAgentSessionStatus info={info} />
						</span>
					</Button>
				</TooltipTrigger>
				<TooltipContent className="max-w-xs break-all">
					{agentLabel}: {info.sessionId}
				</TooltipContent>
			</Tooltip>
			<div className="space-y-2 px-3 pb-3">
				{!expanded &&
					(metadata.unavailable ||
						metadata.resumable ||
						metadata.registrationPending ||
						metadata.resumePending) && (
						<p className="m-0 text-content-secondary wrap-anywhere">
							{metadata.unavailable || metadata.resumable
								? "Session unavailable. Open the terminal for diagnostics, not to restart it."
								: metadata.resumePending
									? "Startup restore pending. Open the terminal to inspect progress."
									: `${metadata.blockedReason || "Waiting for native setup."} Open the terminal to continue setup.`}
						</p>
					)}
				<Button
					size="sm"
					disabled={openDisabled}
					aria-label={`Open ${info.title || agentLabel}`}
					onClick={() => onOpen(info)}
				>
					<TerminalIcon />
					Open
				</Button>
			</div>
		</div>
	);
};

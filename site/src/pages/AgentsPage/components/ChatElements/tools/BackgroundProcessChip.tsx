import { CheckIcon, OctagonXIcon } from "lucide-react";
import type React from "react";
import { useEffect, useState } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { signalTooltipLabel } from "./utils";

type BackgroundProcessChipProps = {
	state: "running" | "exited";
	exitCode?: number;
	killedBySignal?: "kill" | "terminate";
	/** Epoch ms when the process started, for the ticking elapsed time. */
	startedAtMs?: number;
};

const formatElapsed = (ms: number): string => {
	const totalSeconds = Math.max(0, Math.floor(ms / 1000));
	const minutes = Math.floor(totalSeconds / 60);
	const seconds = totalSeconds % 60;
	return `${minutes}:${String(seconds).padStart(2, "0")}`;
};

/**
 * Persistent status affordance for a backgrounded process, shown on
 * the execute row that started it. Replaces the old static
 * "running in background" icon: the chip carries live state
 * (pulsing dot + ticking elapsed time while running, final exit
 * state once observed) so readers can tell at a glance whether
 * anything is still alive.
 */
export const BackgroundProcessChip: React.FC<BackgroundProcessChipProps> = ({
	state,
	exitCode,
	killedBySignal,
	startedAtMs,
}) => {
	const [nowMs, setNowMs] = useState(() => Date.now());

	useEffect(() => {
		if (state !== "running") {
			return;
		}
		const interval = setInterval(() => setNowMs(Date.now()), 1000);
		return () => clearInterval(interval);
	}, [state]);

	if (state === "running") {
		const elapsed =
			startedAtMs !== undefined ? formatElapsed(nowMs - startedAtMs) : null;
		const label = elapsed
			? `Running in background, ${elapsed}`
			: "Running in background";
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						role="status"
						aria-label={label}
						className="flex items-center gap-1.5 rounded-full border border-solid border-border-default px-2 py-0.5 text-2xs leading-none text-content-secondary"
					>
						<span
							aria-hidden
							className="size-1.5 rounded-full bg-content-success animate-pulse motion-reduce:animate-none"
						/>
						running{elapsed ? ` ${elapsed}` : ""}
					</span>
				</TooltipTrigger>
				<TooltipContent>Background process is still running</TooltipContent>
			</Tooltip>
		);
	}

	if (killedBySignal) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						role="status"
						aria-label={signalTooltipLabel(killedBySignal)}
						className="flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-2xs leading-none text-content-secondary"
					>
						<OctagonXIcon aria-hidden className="size-3.5 shrink-0" />
						killed
					</span>
				</TooltipTrigger>
				<TooltipContent>{signalTooltipLabel(killedBySignal)}</TooltipContent>
			</Tooltip>
		);
	}

	const failed = exitCode !== undefined && exitCode !== 0;
	if (!failed) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						role="status"
						aria-label="Background process exited successfully"
						className="flex items-center gap-1 px-0.5 py-0.5 text-content-secondary"
					>
						<CheckIcon aria-hidden className="size-3.5 shrink-0" />
					</span>
				</TooltipTrigger>
				<TooltipContent>Background process exited successfully</TooltipContent>
			</Tooltip>
		);
	}
	return (
		<span
			role="status"
			aria-label={`Background process exited with code ${exitCode}`}
			className="flex items-center gap-1 rounded px-1.5 py-0.5 font-mono text-2xs leading-none bg-surface-red text-content-destructive"
		>
			exit {exitCode}
		</span>
	);
};

import type { FC, ReactNode } from "react";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { isDLPBypassed } from "./dlpBypass";

interface DLPGateProps {
	/**
	 * The user-facing reason this element is gated, or null when it should
	 * render normally. Produced by `dlpDenialReason` / `dlpAppDenialReason`.
	 */
	reason: string | null;
	children: ReactNode;
}

/**
 * Wraps a workspace UI element with a disabled overlay and tooltip when the
 * DLP policy denies the corresponding access path. When `?dlp_bypass=1` is
 * set on the URL, the wrapper is a no-op so the user can click the
 * underlying element and observe the backend 403.
 */
export const DLPGate: FC<DLPGateProps> = ({ reason, children }) => {
	if (!reason || isDLPBypassed()) {
		return <>{children}</>;
	}
	return (
		<TooltipProvider delayDuration={200}>
			<Tooltip>
				<TooltipTrigger asChild>
					<span
						aria-disabled="true"
						tabIndex={-1}
						className="inline-flex"
						data-testid="dlp-gate"
					>
						<span className="pointer-events-none opacity-50" aria-hidden="true">
							{children}
						</span>
					</span>
				</TooltipTrigger>
				<TooltipContent>{reason}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

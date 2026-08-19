import type { FC } from "react";
import { cn } from "#/utils/cn";

type DeviceCodeProps = {
	/** The user code as returned by the device authorization endpoint. */
	userCode: string;
	className?: string;
};

/**
 * Displays a device `user_code` in a large, legible, monospaced style so the
 * user can compare it against the code shown on their device.
 *
 * RFC 8628 §5.4 requires the browser to show the code back to the user before
 * the authorization decision, otherwise an attacker can trick a user into
 * approving a code they never requested.
 */
export const DeviceCode: FC<DeviceCodeProps> = ({ userCode, className }) => {
	return (
		<div
			className={cn(
				`flex items-center justify-center w-full rounded-lg border border-solid
				border-border bg-surface-secondary px-4 py-5`,
				className,
			)}
		>
			{/*
			 * Screen readers get the code spelled out character by character;
			 * sighted users get the large monospaced version.
			 */}
			<span className="sr-only">{userCode.split("").join(" ")}</span>
			<span
				aria-hidden="true"
				className="font-mono text-3xl font-semibold tracking-[0.2em] text-content-primary"
			>
				{userCode}
			</span>
		</div>
	);
};

import { CircleCheckIcon, CircleSlashIcon, ClockIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Welcome } from "#/components/Welcome/Welcome";

export type DeviceAuthResult = "approved" | "denied" | "expired";

type DeviceAuthResultViewProps = {
	result: DeviceAuthResult;
	clientName: string;
	/** Only used by the `expired` state, which is recoverable in place. */
	onStartOver?: () => void;
};

/**
 * Icon and color are paired to the state, and never the only signal — the
 * heading and body text say the same thing.
 */
const resultContent = {
	approved: {
		icon: CircleCheckIcon,
		iconClassName: "text-content-success",
		title: "Device connected",
		body: (clientName: string) =>
			`${clientName} can now access Coder on your behalf. Return to your device to continue — it picks up the result on its own.`,
	},
	denied: {
		icon: CircleSlashIcon,
		iconClassName: "text-content-destructive",
		title: "Device not connected",
		body: (clientName: string) =>
			`${clientName} was not given access, and nothing was shared. Your device will stop waiting shortly.`,
	},
	expired: {
		icon: ClockIcon,
		iconClassName: "text-content-warning",
		title: "Code expired",
		body: () =>
			"This code expired before it was confirmed. Codes last a few minutes — start again on your device to get a new one.",
	},
} as const;

/**
 * Step 3 of the device authorization flow: the terminal screen. There is no
 * redirect — the CLI or app polls and picks up the result itself — so each
 * state ends by telling the user the tab is safe to close.
 *
 * Every state still offers one in-app destination, so the page is never a dead
 * end.
 */
export const DeviceAuthResultView: FC<DeviceAuthResultViewProps> = ({
	result,
	clientName,
	onStartOver,
}) => {
	const content = resultContent[result];
	const Icon = content.icon;
	const isExpired = result === "expired";

	return (
		<SignInLayout>
			<main className="w-full flex flex-col items-center gap-6">
				<div className="flex flex-col items-center gap-2">
					<Welcome>{content.title}</Welcome>
					<Icon
						aria-hidden="true"
						className={`size-icon-lg ${content.iconClassName}`}
					/>
				</div>

				<div className="flex flex-col gap-2">
					<p className="m-0 text-center text-sm text-content-secondary">
						{content.body(clientName)}
					</p>
					<p className="m-0 text-center text-sm text-content-secondary">
						You can close this tab.
					</p>
				</div>

				<div className="w-full flex flex-col gap-2">
					{isExpired && onStartOver && (
						<Button className="w-full" onClick={onStartOver}>
							Enter a new code
						</Button>
					)}
					<Button
						className="w-full"
						variant={isExpired ? "outline" : "default"}
						asChild
					>
						<RouterLink to="/workspaces">Go to workspaces</RouterLink>
					</Button>
				</div>
			</main>
		</SignInLayout>
	);
};

import { CircleCheckIcon, CircleSlashIcon, ClockIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Welcome } from "#/components/Welcome/Welcome";

export type DeviceAuthResult = "approved" | "denied" | "expired";

type DeviceAuthResultViewProps = {
	result: DeviceAuthResult;
	clientName: string;
	/** Only rendered for the `expired` state, which is recoverable. */
	onStartOver?: () => void;
};

const resultContent = {
	approved: {
		icon: CircleCheckIcon,
		iconClassName: "text-content-success",
		title: "Device connected",
		body: (clientName: string) =>
			`${clientName} now has access to your Coder account. Return to your device to continue — it picks up the result automatically.`,
	},
	denied: {
		icon: CircleSlashIcon,
		iconClassName: "text-content-destructive",
		title: "Access denied",
		body: (clientName: string) =>
			`${clientName} was not given access to your Coder account. Nothing was shared.`,
	},
	expired: {
		icon: ClockIcon,
		iconClassName: "text-content-warning",
		title: "Code expired",
		body: () =>
			"This code expired before it was confirmed. Start the flow again on your device to get a new code.",
	},
} as const;

/**
 * Step 3 of the device authorization flow: the terminal screen. There is no
 * redirect — the CLI or app is polling and picks up the result on its own, so
 * every state ends with "you can close this tab".
 */
export const DeviceAuthResultView: FC<DeviceAuthResultViewProps> = ({
	result,
	clientName,
	onStartOver,
}) => {
	const content = resultContent[result];
	const Icon = content.icon;

	return (
		<SignInLayout>
			<Welcome>{content.title}</Welcome>

			<Icon className={`size-8 mt-4 ${content.iconClassName}`} />

			<p className="m-0 mt-4 text-center text-sm text-content-secondary leading-normal">
				{content.body(clientName)}
			</p>

			<p className="m-0 mt-2 text-center text-sm text-content-secondary leading-normal">
				You can close this tab.
			</p>

			{result === "expired" && onStartOver && (
				<Button size="lg" className="w-full mt-6" onClick={onStartOver}>
					Enter a new code
				</Button>
			)}
		</SignInLayout>
	);
};

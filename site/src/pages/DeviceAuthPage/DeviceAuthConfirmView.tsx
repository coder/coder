import { KeyRoundIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { Welcome } from "#/components/Welcome/Welcome";
import { DeviceCode } from "./DeviceCode";

export type DeviceAuthScope = {
	/** The raw scope string, e.g. `workspace:read`. */
	name: string;
	/** Human readable description of what the scope allows. */
	description: string;
};

type DeviceAuthConfirmViewProps = {
	userCode: string;
	clientName: string;
	clientIcon?: string;
	scopes: readonly DeviceAuthScope[];
	username: string;
	isSubmitting?: boolean;
	onApprove: () => void;
	onDeny: () => void;
};

/**
 * Step 2 of the device authorization flow: the authorization decision. Shows
 * the code back to the user (RFC 8628 §5.4), what is requesting access, and the
 * exact permissions being granted.
 *
 * Approve is the only `default` button; Deny is `outline`. Deny is not
 * `destructive` — the user can start the flow again from their device, so
 * nothing here is irreversible.
 */
export const DeviceAuthConfirmView: FC<DeviceAuthConfirmViewProps> = ({
	userCode,
	clientName,
	clientIcon,
	scopes,
	username,
	isSubmitting = false,
	onApprove,
	onDeny,
}) => {
	return (
		<SignInLayout>
			<main className="w-full flex flex-col gap-6">
				<div className="flex flex-col gap-2">
					{/*
					 * The heading stays fixed rather than interpolating the client
					 * name: a long name wrapped to three lines and pushed the layout
					 * past the viewport. The requester is named in the card below,
					 * truncated with the full value recoverable.
					 */}
					<Welcome>Approve access</Welcome>
					<p className="m-0 text-center text-sm text-content-secondary">
						Confirm this matches the code shown on your device.
					</p>
				</div>

				<DeviceCode userCode={userCode} />

				<div className="flex flex-col gap-4 rounded-md border border-solid border-border p-4">
					<div className="flex items-center gap-3">
						<div className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-solid border-border bg-surface-secondary">
							{clientIcon ? (
								<img
									src={clientIcon}
									alt=""
									className="size-full object-contain p-1"
								/>
							) : (
								<KeyRoundIcon
									aria-hidden="true"
									className="size-icon-sm text-content-secondary"
								/>
							)}
						</div>
						<div className="flex min-w-0 flex-col">
							{/* Truncated rather than wrapped, with the full name recoverable. */}
							<Tooltip>
								{/*
								 * Left as the default button trigger rather than `asChild` on a
								 * span, so the truncated value is reachable by keyboard. The
								 * repo's a11y lint rejects tabIndex on non-interactive elements.
								 */}
								<TooltipTrigger className="block w-full truncate border-none bg-transparent p-0 text-left text-sm text-content-primary">
									{clientName}
								</TooltipTrigger>
								<TooltipContent>{clientName}</TooltipContent>
							</Tooltip>
							<span className="text-xs text-content-secondary">
								is requesting access to your account
							</span>
						</div>
					</div>

					<div className="flex flex-col gap-2">
						<span className="text-xs text-content-secondary">
							Permissions requested
						</span>
						<ul className="m-0 flex list-none flex-col gap-2 p-0">
							{scopes.map((scope) => (
								<li key={scope.name} className="flex flex-col">
									<span className="text-sm text-content-primary">
										{scope.description}
									</span>
									{/* Mono because the scope is an identifier. */}
									<span className="font-mono text-2xs text-content-secondary">
										{scope.name}
									</span>
								</li>
							))}
						</ul>
					</div>
				</div>

				<p className="m-0 text-center text-xs text-content-secondary">
					Access is granted as {username}. If this didn't come from your device,
					deny it.
				</p>

				<div className="flex flex-col gap-2">
					<Button
						className="w-full"
						disabled={isSubmitting}
						onClick={onApprove}
					>
						<Spinner loading={isSubmitting} />
						{isSubmitting ? "Connecting…" : "Approve"}
					</Button>
					<Button
						variant="outline"
						className="w-full"
						disabled={isSubmitting}
						onClick={onDeny}
					>
						Deny
					</Button>
				</div>
			</main>
		</SignInLayout>
	);
};

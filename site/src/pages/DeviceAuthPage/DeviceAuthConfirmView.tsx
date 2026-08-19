import { KeyRoundIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
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
			<Welcome>Authorize {clientName}</Welcome>

			<p className="m-0 text-center text-sm text-content-secondary leading-normal">
				Confirm this matches the code shown on your device.
			</p>

			<DeviceCode userCode={userCode} className="mt-4" />

			<div className="w-full mt-6 flex flex-col gap-4 rounded-lg border border-solid border-border p-4">
				<div className="flex items-center gap-3">
					<div className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-md border border-solid border-border bg-surface-secondary text-content-secondary">
						{clientIcon ? (
							<img
								src={clientIcon}
								alt=""
								className="size-full object-contain p-1"
							/>
						) : (
							<KeyRoundIcon className="size-icon-sm" />
						)}
					</div>
					<div className="flex flex-col">
						<span className="text-sm font-semibold text-content-primary">
							{clientName}
						</span>
						<span className="text-xs text-content-secondary">
							is requesting access to your account
						</span>
					</div>
				</div>

				<div className="flex flex-col gap-2">
					<span className="text-xs font-medium uppercase tracking-wide text-content-secondary">
						Permissions requested
					</span>
					<ul className="m-0 flex list-none flex-col gap-2 p-0">
						{scopes.map((scope) => (
							<li key={scope.name} className="flex flex-col">
								<span className="text-sm text-content-primary">
									{scope.description}
								</span>
								<span className="font-mono text-2xs text-content-secondary">
									{scope.name}
								</span>
							</li>
						))}
					</ul>
				</div>
			</div>

			<p className="m-0 mt-4 text-center text-xs text-content-secondary leading-normal">
				Access is granted as{" "}
				<strong className="font-medium text-content-primary">{username}</strong>
				. If you didn't start this from your device, deny it.
			</p>

			<div className="w-full mt-4 flex flex-col gap-2">
				<Button
					size="lg"
					className="w-full"
					disabled={isSubmitting}
					onClick={onApprove}
				>
					<Spinner loading={isSubmitting} />
					Approve
				</Button>
				<Button
					size="lg"
					variant="outline"
					className="w-full"
					disabled={isSubmitting}
					onClick={onDeny}
				>
					Deny
				</Button>
			</div>
		</SignInLayout>
	);
};

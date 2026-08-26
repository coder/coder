import { type FC, useId, useRef, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Spinner } from "#/components/Spinner/Spinner";

type DynamicClientRegistrationSettingProps = {
	enabled: boolean;
	canEdit: boolean;
	isUpdating: boolean;
	onChange: (enabled: boolean) => void;
};

export const DynamicClientRegistrationSetting: FC<
	DynamicClientRegistrationSettingProps
> = ({ enabled, canEdit, isUpdating, onChange }) => {
	const headingId = useId();
	const [isEnableDialogOpen, setIsEnableDialogOpen] = useState(false);
	const buttonRef = useRef<HTMLButtonElement>(null);

	return (
		<>
			<section
				aria-labelledby={headingId}
				className="flex flex-row items-start justify-between gap-8"
			>
				<div className="flex flex-col gap-1 max-w-xl">
					<div className="flex flex-row items-center gap-2">
						<h2
							id={headingId}
							className="text-content-primary text-base font-semibold m-0"
						>
							Dynamic Client Registration
						</h2>
						{enabled && (
							<Badge size="sm" variant="green" className="border-0 shadow-none">
								Enabled
							</Badge>
						)}
					</div>
					{/*
					 * Disabling only gates the registration endpoint. It deletes no apps,
					 * secrets, or tokens, so the caveat stays visible in both states: an
					 * admin who has just disabled needs it as much as one deciding to.
					 */}
					<p className="text-sm text-content-secondary m-0">
						Allow OAuth2 clients to register themselves at{" "}
						<code className="text-xs">/oauth2/register</code> without prior
						administrator approval (RFC 7591). Disabling stops new
						registrations. Clients that already registered keep working until an
						administrator deletes them.
					</p>
					{/*
					 * A disabled button takes no focus and no pointer events, so it
					 * cannot carry the reason it is disabled. Stating the reason here
					 * puts it in reading order ahead of the button for everyone.
					 */}
					{!canEdit && (
						<p className="text-sm text-content-secondary m-0 mt-1">
							You need permission to edit deployment configuration to change
							this setting.
						</p>
					)}
				</div>

				{/*
				 * Lacking permission is permanent, so the button is genuinely
				 * unavailable and takes the native attribute. An in-flight request is
				 * momentary and the button is where focus already is, so it goes inert
				 * without leaving the tab order: disabling a focused element blurs it,
				 * which drops a keyboard user back to the top of the document mid-flip.
				 */}
				<Button
					ref={buttonRef}
					variant={enabled ? "outline-solid" : "default"}
					disabled={!canEdit}
					aria-disabled={isUpdating}
					className="aria-disabled:pointer-events-none"
					onClick={() => {
						if (isUpdating) {
							return;
						}
						if (enabled) {
							onChange(false);
						} else {
							setIsEnableDialogOpen(true);
						}
					}}
				>
					<Spinner loading={isUpdating} aria-hidden />
					{enabled ? "Disable" : "Enable"}
				</Button>
			</section>

			<ConfirmDialog
				type="delete"
				open={isEnableDialogOpen}
				onConfirm={() => {
					setIsEnableDialogOpen(false);
					onChange(true);
				}}
				onClose={() => setIsEnableDialogOpen(false)}
				// Radix returns focus to its trigger on close, and this dialog has
				// none, so focus would land on <body>. Focusing from the handlers
				// above does not survive: Radix moves focus again when the exit
				// animation ends.
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					buttonRef.current?.focus();
				}}
				title="Enable Dynamic Client Registration?"
				confirmText="Enable"
				description="Any client that can reach this deployment will be able to register itself as an OAuth2 application, with no Coder account and no administrator approval. Disabling later blocks new registrations but does not revoke clients that already registered."
			/>
		</>
	);
};

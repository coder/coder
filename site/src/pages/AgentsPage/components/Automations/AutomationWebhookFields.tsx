import { useId, useRef, useState } from "react";
import { getErrorMessage, isApiError } from "#/api/errors";
import { webhookPublishEndpoint } from "#/api/queries/chatAutomations";
import type {
	ChatAutomation,
	ChatAutomationWebhookUse,
} from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { restoreFocusTo } from "#/hooks/useRestoreFocusOnClose";
import { formatDate } from "#/utils/time";
import { RadioCards } from "./RadioCards";

const webhookUseOptions = [
	{ value: "multi", label: "Multi-use" },
	{ value: "single", label: "Single-use" },
] as const;

type AutomationWebhookFieldsProps = {
	/** Shows this saved webhook; shows the Use choice when unset. */
	automation?: ChatAutomation;
	origin: string;
	webhookUse: ChatAutomationWebhookUse;
	onWebhookUseChange: (webhookUse: ChatAutomationWebhookUse) => void;
	rotateSecretError: unknown;
	isRotatingSecret: boolean;
	isSubmitting: boolean;
	/** False for viewers who do not own the webhook. */
	canRotateSecret: boolean;
	onRotateSecret: (rotateButton: HTMLButtonElement | null) => void;
};

export const AutomationWebhookFields: React.FC<
	AutomationWebhookFieldsProps
> = ({
	automation,
	origin,
	webhookUse,
	onWebhookUseChange,
	rotateSecretError,
	isRotatingSecret,
	isSubmitting,
	canRotateSecret,
	onRotateSecret,
}) => {
	const useLabelId = useId();
	const useHelpId = useId();
	const rotateButtonRef = useRef<HTMLButtonElement>(null);
	const [confirmingRotate, setConfirmingRotate] = useState(false);

	const webhookUseField = (
		<div className="flex flex-col gap-2">
			<span
				id={useLabelId}
				className="text-sm font-medium text-content-primary"
			>
				Use
			</span>
			<RadioCards
				aria-labelledby={useLabelId}
				aria-describedby={useHelpId}
				value={webhookUse}
				disabled={Boolean(automation)}
				options={webhookUseOptions}
				onValueChange={onWebhookUseChange}
			/>
			<span id={useHelpId} className="text-xs text-content-secondary">
				{webhookUse === "single"
					? "A single-use webhook accepts one event, then rejects the rest."
					: "A multi-use webhook accepts every event while the automation is on."}
			</span>
		</div>
	);
	const endpointHelp = (
		<span className="text-xs text-content-secondary">
			POST any JSON body with the automation secret as a Bearer token.
			{automation
				? " Rotate the secret to get a new one."
				: " The secret is shown once after saving."}
		</span>
	);
	if (!automation) {
		return (
			<>
				{webhookUseField}
				<div className="flex flex-col gap-2">
					<span className="text-sm font-medium text-content-primary">
						Endpoint
					</span>
					{/* The ID exists only after saving, so there is nothing to copy. */}
					<div className="break-all rounded-md border border-solid border-border bg-surface-secondary px-3 py-2.5 font-mono text-xs text-content-secondary">
						{/* Decoded so the placeholder reads <automation-id>, not %3C...%3E. */}
						{decodeURIComponent(
							webhookPublishEndpoint(origin, "<automation-id>"),
						)}
					</div>
					{endpointHelp}
				</div>
			</>
		);
	}

	// A used single-use webhook rejects every event, so a new secret is useless.
	const isUsedUp =
		automation.webhook_use === "single" &&
		Boolean(automation.webhook_consumed_at);
	// getErrorDetail would add a developer-console hint the server never sent.
	const rotateErrorDetail = isApiError(rotateSecretError)
		? rotateSecretError.response.data.detail
		: undefined;
	return (
		<>
			{webhookUseField}
			{automation.webhook_consumed_at && (
				<p className="m-0 text-sm text-content-secondary">
					Used on{" "}
					{formatDate(new Date(automation.webhook_consumed_at), {
						locale: "en-US",
						timeZoneName: "short",
					})}
				</p>
			)}
			<div className="flex flex-col gap-2">
				<span className="text-sm font-medium text-content-primary">
					Endpoint
				</span>
				<CodeExample
					secret={false}
					code={webhookPublishEndpoint(origin, automation.id)}
					copyLabel="Copy endpoint"
				/>
				{endpointHelp}
			</div>
			{/* Next to the button so it stays in view in a scrolled form. */}
			{Boolean(rotateSecretError) && (
				<Alert severity="error" prominent>
					<AlertTitle>
						{getErrorMessage(
							rotateSecretError,
							"Could not rotate the webhook secret.",
						)}
					</AlertTitle>
					{rotateErrorDetail && (
						<AlertDescription>{rotateErrorDetail}</AlertDescription>
					)}
				</Alert>
			)}
			{canRotateSecret && !isUsedUp && (
				<Button
					ref={rotateButtonRef}
					type="button"
					variant="outline"
					size="sm"
					className="w-fit aria-disabled:cursor-not-allowed aria-disabled:text-content-disabled"
					// Stays focusable while rotating so focus can return here.
					disabled={isSubmitting}
					aria-disabled={isRotatingSecret}
					onClick={() => {
						if (!isRotatingSecret) {
							setConfirmingRotate(true);
						}
					}}
				>
					<Spinner loading={isRotatingSecret} />
					Rotate secret
				</Button>
			)}
			<ConfirmDialog
				open={confirmingRotate}
				type="delete"
				title="Rotate the webhook secret?"
				description="The current secret stops working immediately."
				confirmText="Rotate secret"
				onClose={() => setConfirmingRotate(false)}
				onConfirm={() => {
					setConfirmingRotate(false);
					onRotateSecret(rotateButtonRef.current);
				}}
				// The button, not the focused element: Safari does not focus a
				// button on click.
				onCloseAutoFocus={(event) =>
					restoreFocusTo(event, rotateButtonRef.current)
				}
			/>
		</>
	);
};

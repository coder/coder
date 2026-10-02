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
import { RadioGroup } from "#/components/RadioGroup/RadioGroup";
import { Spinner } from "#/components/Spinner/Spinner";
import { useRestoreFocusOnClose } from "#/hooks/useRestoreFocusOnClose";
import { formatDate } from "#/utils/time";
import { RadioOption } from "./RadioOption";

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
	const restoreFocus = useRestoreFocusOnClose(confirmingRotate);

	const useField = (
		<div className="flex flex-col gap-2">
			<span
				id={useLabelId}
				className="text-sm font-medium text-content-primary"
			>
				Use
			</span>
			<RadioGroup
				aria-labelledby={useLabelId}
				aria-describedby={useHelpId}
				value={automation?.webhook_use ?? webhookUse}
				disabled={Boolean(automation)}
				onValueChange={(value) => {
					if (value === "single" || value === "multi") {
						onWebhookUseChange(value);
					}
				}}
			>
				<RadioOption value="multi" label="Multi-use" />
				<RadioOption value="single" label="Single-use" />
			</RadioGroup>
			<span id={useHelpId} className="text-xs text-content-secondary">
				A single-use webhook accepts one event.
			</span>
		</div>
	);
	if (!automation) {
		return useField;
	}

	const isSingleUse = automation.webhook_use === "single";
	// A used single-use webhook rejects every event, so a new secret is useless.
	const isUsedUp = isSingleUse && Boolean(automation.webhook_consumed_at);
	// getErrorDetail would add a developer-console hint the server never sent.
	const rotateErrorDetail = isApiError(rotateSecretError)
		? rotateSecretError.response.data.detail
		: undefined;
	return (
		<>
			{useField}
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
					Publish endpoint
				</span>
				<CodeExample
					secret={false}
					code={webhookPublishEndpoint(origin, automation.id)}
					copyLabel="Copy endpoint"
				/>
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
				onCloseAutoFocus={restoreFocus}
			/>
		</>
	);
};

import { type FC, useState } from "react";
import { useSearchParams } from "react-router";
import { pageTitle } from "#/utils/page";
import {
	DeviceAuthConfirmView,
	type DeviceAuthScope,
} from "./DeviceAuthConfirmView";
import {
	DeviceAuthEnterCodeView,
	type DeviceCodeError,
} from "./DeviceAuthEnterCodeView";
import {
	type DeviceAuthResult,
	DeviceAuthResultView,
} from "./DeviceAuthResultView";

type Step =
	| { name: "enter-code"; error?: DeviceCodeError }
	| { name: "confirm" }
	| { name: "result"; result: DeviceAuthResult };

/**
 * Device authorization grant (RFC 8628) browser flow. Standalone page, no
 * dashboard chrome, registered at `/device`.
 *
 * 1. Enter code — skipped when the user arrives via `verification_uri_complete`
 *    with a `user_code` query parameter.
 * 2. Confirm — shows the code back to the user plus the requested scopes, and
 *    is the authorization decision point.
 * 3. Result — terminal screen. No redirect; the device polls for the outcome.
 *
 * TODO(oauth2-device-flow): the provider side of the grant does not exist yet
 * (`coderd/oauth2provider/tokens.go`: "TODO: Client creds, device code"). The
 * verification lookup and approve/deny calls below are the integration points;
 * the client name and scopes come from the device code record.
 */
const DeviceAuthPage: FC = () => {
	const [searchParams] = useSearchParams();
	const prefilledUserCode = searchParams.get("user_code") ?? "";

	const [step, setStep] = useState<Step>(
		prefilledUserCode ? { name: "confirm" } : { name: "enter-code" },
	);
	const [userCode, setUserCode] = useState(prefilledUserCode);

	// Placeholder until the API exposes the device code record.
	const clientName = "Coder CLI";
	const scopes: readonly DeviceAuthScope[] = [];
	const username = "";

	return (
		<>
			<title>{pageTitle("Authorize device")}</title>

			{step.name === "enter-code" && (
				<DeviceAuthEnterCodeView
					initialUserCode={userCode}
					error={step.error}
					onSubmit={(submittedCode) => {
						setUserCode(submittedCode);
						setStep({ name: "confirm" });
					}}
				/>
			)}

			{step.name === "confirm" && (
				<DeviceAuthConfirmView
					userCode={userCode}
					clientName={clientName}
					scopes={scopes}
					username={username}
					onApprove={() => setStep({ name: "result", result: "approved" })}
					onDeny={() => setStep({ name: "result", result: "denied" })}
				/>
			)}

			{step.name === "result" && (
				<DeviceAuthResultView
					result={step.result}
					clientName={clientName}
					onStartOver={() => setStep({ name: "enter-code" })}
				/>
			)}
		</>
	);
};

export default DeviceAuthPage;

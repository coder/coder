import { type FC, type FormEvent, useState } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
import { Welcome } from "#/components/Welcome/Welcome";

/**
 * Errors the server can return for a submitted code. Client-side format
 * problems are handled inline as field validation instead.
 */
export type DeviceCodeError = "not-recognized" | "expired";

type DeviceAuthEnterCodeViewProps = {
	/** Pre-fills the field when the user arrives via `verification_uri_complete`. */
	initialUserCode?: string;
	isSubmitting?: boolean;
	error?: DeviceCodeError;
	onSubmit: (userCode: string) => void;
};

// Factual, non-accusatory: the code isn't recognized, the user isn't at fault.
const errorMessages: Record<DeviceCodeError, string> = {
	"not-recognized":
		"This code isn't recognized. It may have already been used — start again on your device to get a new one.",
	expired:
		"This code has expired. Codes last a few minutes, so start again on your device to get a new one.",
};

/**
 * Formats free-form input into the `XXXX-XXXX` shape used by device codes, so
 * the user can type with or without the separator.
 */
const formatUserCode = (value: string): string => {
	const characters = value
		.toUpperCase()
		.replace(/[^A-Z0-9]/g, "")
		.slice(0, 8);
	if (characters.length <= 4) {
		return characters;
	}
	return `${characters.slice(0, 4)}-${characters.slice(4)}`;
};

/**
 * Step 1 of the device authorization flow: the user types the code shown by
 * the CLI or app. Standalone page, no navigation chrome — nothing else here is
 * reachable until the flow completes.
 */
export const DeviceAuthEnterCodeView: FC<DeviceAuthEnterCodeViewProps> = ({
	initialUserCode = "",
	isSubmitting = false,
	error,
	onSubmit,
}) => {
	const [userCode, setUserCode] = useState(formatUserCode(initialUserCode));
	// Validation runs on submit, not on keystroke: the submit button stays
	// enabled so the form can fail loudly and say what's missing.
	const [formatError, setFormatError] = useState<string>();

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (isSubmitting) {
			return;
		}
		if (userCode.replace("-", "").length !== 8) {
			setFormatError("Enter all eight characters of the code, like WDJB-MJHT.");
			return;
		}
		setFormatError(undefined);
		onSubmit(userCode);
	};

	const fieldError = formatError;

	return (
		<SignInLayout>
			<main className="w-full flex flex-col gap-6">
				<div className="flex flex-col gap-2">
					<Welcome>Connect your device</Welcome>
					<p className="m-0 text-center text-sm text-content-secondary">
						Enter the code shown in your terminal or app.
					</p>
				</div>

				{/* Alert carries the server-side condition; it is subtle by default. */}
				{error && <Alert severity="error">{errorMessages[error]}</Alert>}

				<form
					className="flex flex-col gap-6"
					onSubmit={handleSubmit}
					noValidate
				>
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="user-code">Device code</Label>
						<Input
							id="user-code"
							name="user_code"
							autoFocus
							autoComplete="off"
							spellCheck={false}
							placeholder="WDJB-MJHT"
							value={userCode}
							onChange={(event) =>
								setUserCode(formatUserCode(event.target.value))
							}
							aria-invalid={fieldError ? true : undefined}
							aria-describedby={fieldError ? "user-code-error" : undefined}
							className="text-center font-mono"
						/>
						{fieldError && (
							<p
								id="user-code-error"
								className="m-0 text-xs text-content-destructive"
							>
								{fieldError}
							</p>
						)}
					</div>

					<Button type="submit" className="w-full" disabled={isSubmitting}>
						<Spinner loading={isSubmitting} />
						{isSubmitting ? "Checking…" : "Continue"}
					</Button>
				</form>
			</main>
		</SignInLayout>
	);
};

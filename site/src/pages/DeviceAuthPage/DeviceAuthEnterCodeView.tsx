import { type FC, type FormEvent, useState } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Spinner } from "#/components/Spinner/Spinner";
import { Welcome } from "#/components/Welcome/Welcome";

export type DeviceCodeError = "invalid" | "expired";

type DeviceAuthEnterCodeViewProps = {
	/** Pre-fills the field when the user arrives via `verification_uri_complete`. */
	initialUserCode?: string;
	isSubmitting?: boolean;
	error?: DeviceCodeError;
	onSubmit: (userCode: string) => void;
};

const errorMessages: Record<DeviceCodeError, string> = {
	invalid: "That code isn't valid. Check for typos and try again.",
	expired:
		"That code has expired. Codes are only valid for a few minutes — start over on your device to get a new one.",
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
 * the CLI or app. Rendered as a standalone page with no dashboard chrome.
 */
export const DeviceAuthEnterCodeView: FC<DeviceAuthEnterCodeViewProps> = ({
	initialUserCode = "",
	isSubmitting = false,
	error,
	onSubmit,
}) => {
	const [userCode, setUserCode] = useState(formatUserCode(initialUserCode));
	const isComplete = userCode.replace("-", "").length === 8;

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (isComplete && !isSubmitting) {
			onSubmit(userCode);
		}
	};

	return (
		<SignInLayout>
			<Welcome>Enter device code</Welcome>

			<p className="m-0 text-center text-sm text-content-secondary leading-normal">
				Enter the code shown in your terminal or app to connect it to your Coder
				account.
			</p>

			<form className="w-full mt-6 flex flex-col gap-4" onSubmit={handleSubmit}>
				{error && (
					<Alert severity="error" prominent>
						{errorMessages[error]}
					</Alert>
				)}

				<div className="flex flex-col gap-2">
					<Label htmlFor="user-code">Device code</Label>
					<Input
						id="user-code"
						name="user_code"
						autoFocus
						autoComplete="off"
						spellCheck={false}
						placeholder="XXXX-XXXX"
						aria-invalid={error !== undefined}
						aria-describedby="user-code-help"
						value={userCode}
						onChange={(event) =>
							setUserCode(formatUserCode(event.target.value))
						}
						className="text-center font-mono text-base tracking-[0.2em] uppercase"
					/>
					<span
						id="user-code-help"
						className="text-xs text-content-secondary leading-normal"
					>
						Eight characters, letters and numbers only.
					</span>
				</div>

				<Button
					type="submit"
					size="lg"
					className="w-full"
					disabled={!isComplete || isSubmitting}
				>
					<Spinner loading={isSubmitting} />
					Continue
				</Button>
			</form>
		</SignInLayout>
	);
};

import { type FC, type FormEvent, useState } from "react";
import { Alert } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import type { OAuth2ClientType } from "./ClientTypeBadge";

type OAuth2ClientFormViewProps = {
	initialType?: OAuth2ClientType;
	isSubmitting?: boolean;
	onSubmit: (values: {
		name: string;
		callbackUrl: string;
		type: OAuth2ClientType;
	}) => void;
	onCancel: () => void;
};

const typeOptions: {
	value: OAuth2ClientType;
	label: string;
	description: string;
}[] = [
	{
		value: "confidential",
		label: "Confidential",
		description:
			"Runs on a server that can keep a secret — a web backend or an internal service. Authenticates with a client secret.",
	},
	{
		value: "public",
		label: "Public",
		description:
			"Runs on a user's machine — a CLI, desktop app, mobile app or SPA. Can't keep a secret, so it uses PKCE instead.",
	},
];

/**
 * Registering an OAuth2 client (Flow 3 — PLAT-504).
 *
 * Client type is chosen here rather than being derived later, because it
 * determines whether the client has a secret at all. Two options that need
 * comparing is exactly the RadioGroup case — a Select would hide the
 * distinction the admin is being asked to make.
 *
 * The consequence of the choice is stated inline as it's made, rather than
 * discovered on the next screen when the secret field they expected isn't there.
 */
export const OAuth2ClientFormView: FC<OAuth2ClientFormViewProps> = ({
	initialType = "confidential",
	isSubmitting = false,
	onSubmit,
	onCancel,
}) => {
	const [name, setName] = useState("");
	const [callbackUrl, setCallbackUrl] = useState("");
	const [type, setType] = useState<OAuth2ClientType>(initialType);
	const [errors, setErrors] = useState<{ name?: string; callbackUrl?: string }>(
		{},
	);

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		// Validation runs on submit; the button is never disabled as the way of
		// signalling that something is missing.
		const nextErrors: typeof errors = {};
		if (name.trim() === "") {
			nextErrors.name = "Enter a name for this application.";
		}
		if (callbackUrl.trim() === "") {
			nextErrors.callbackUrl = "Enter the URL Coder should redirect back to.";
		}
		setErrors(nextErrors);
		if (Object.keys(nextErrors).length > 0) {
			return;
		}
		onSubmit({ name, callbackUrl, type });
	};

	return (
		<div className="flex flex-col gap-6 max-w-2xl">
			<SettingsHeader>
				<SettingsHeaderTitle>Add application</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Register an application that can request access to Coder on behalf of
					your users.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<form onSubmit={handleSubmit} noValidate className="flex flex-col gap-6">
				<h2 className="m-0 text-base font-semibold">General</h2>

				<div className="flex flex-col gap-5">
					<div className="flex flex-col gap-1.5">
						<Label htmlFor="client-name">Name</Label>
						<Input
							id="client-name"
							value={name}
							placeholder="My integration"
							onChange={(event) => setName(event.target.value)}
							aria-invalid={errors.name ? true : undefined}
							aria-describedby={errors.name ? "client-name-error" : undefined}
						/>
						{errors.name && (
							<p
								id="client-name-error"
								className="m-0 text-xs text-content-destructive"
							>
								{errors.name}
							</p>
						)}
					</div>

					<div className="flex flex-col gap-1.5">
						<Label htmlFor="callback-url">Callback URL</Label>
						<Input
							id="callback-url"
							value={callbackUrl}
							placeholder="https://example.com/oauth/callback"
							onChange={(event) => setCallbackUrl(event.target.value)}
							aria-invalid={errors.callbackUrl ? true : undefined}
							aria-describedby={
								errors.callbackUrl ? "callback-url-error" : undefined
							}
						/>
						{errors.callbackUrl && (
							<p
								id="callback-url-error"
								className="m-0 text-xs text-content-destructive"
							>
								{errors.callbackUrl}
							</p>
						)}
					</div>
				</div>

				<h2 className="m-0 text-base font-semibold">Client type</h2>

				<div className="flex flex-col gap-4">
					<RadioGroup
						value={type}
						onValueChange={(value) => setType(value as OAuth2ClientType)}
						aria-label="Client type"
						className="gap-3"
					>
						{typeOptions.map((option) => (
							<div key={option.value} className="flex items-start gap-2">
								<RadioGroupItem
									id={`client-type-${option.value}`}
									value={option.value}
									className="mt-1"
								/>
								<div className="flex flex-col gap-0.5">
									<Label
										htmlFor={`client-type-${option.value}`}
										className="cursor-pointer"
									>
										{option.label}
									</Label>
									<span className="text-xs text-content-secondary">
										{option.description}
									</span>
								</div>
							</div>
						))}
					</RadioGroup>

					{/*
					 * Subtle, not prominent: this explains a consequence of the field
					 * above it, it isn't a page-level condition.
					 */}
					<Alert severity="info">
						{type === "public"
							? "Public clients don't get a client secret. They prove the authorization request came from them using PKCE, which is generated per request and can't be extracted from the app."
							: "Coder will generate a client secret after you save. Store it somewhere safe — it's shown once."}
					</Alert>
				</div>

				<div className="flex items-center justify-end gap-2 border-t border-solid border-border pt-4">
					<Button type="button" variant="outline" onClick={onCancel}>
						Cancel
					</Button>
					<Button type="submit" disabled={isSubmitting}>
						<Spinner loading={isSubmitting} />
						{isSubmitting ? "Adding…" : "Add application"}
					</Button>
				</div>
			</form>
		</div>
	);
};

import { cn } from "cn";
import { type FormikHelpers, useFormik } from "formik";
import { ShieldCheckIcon } from "lucide-react";
import { useId, useRef, useState } from "react";
import type { ChatProjectTrigger } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Switch } from "#/components/Switch/Switch";
import { OptionCheckbox } from "./OptionCheckbox";
import type { TriggerChipProps } from "./TriggerSentenceChip";
import {
	type TriggerSecretInfo,
	triggerEventOptions,
	triggerSecretMaxLength,
	triggerSources,
} from "./triggerCatalog";

type EventChecklistProps = {
	readonly trigger: ChatProjectTrigger;
	readonly onUpdate: TriggerChipProps["onUpdate"];
};

/** "Any event" plus one checkbox per event. Each change saves at once. */
export const EventChecklist: React.FC<EventChecklistProps> = ({
	trigger,
	onUpdate,
}) => (
	<fieldset className="m-0 flex flex-col gap-1 border-0 p-0">
		<legend className="mb-1 p-0 text-xs font-medium text-content-secondary">
			Start a chat on
		</legend>
		<OptionCheckbox
			label={triggerSources[trigger.source].anyEventLabel}
			checked={trigger.events.length === 0}
			onCheckedChange={() => onUpdate({ events: [] })}
		/>
		{triggerEventOptions(trigger.source, trigger.events).map((option) => (
			<OptionCheckbox
				key={option.value}
				label={option.optionLabel}
				checked={trigger.events.includes(option.value)}
				onCheckedChange={(checked) =>
					onUpdate({
						events: checked
							? [...trigger.events, option.value]
							: trigger.events.filter((event) => event !== option.value),
					})
				}
			/>
		))}
	</fieldset>
);

type TextPatternFieldsProps = {
	readonly text: string;
	readonly isRegex: boolean;
	/** Why the text cannot be saved, shown in place of the hint. */
	readonly error: string | undefined;
	readonly onTextChange: (text: string) => void;
	readonly onRegexChange: (isRegex: boolean) => void;
	readonly onSubmit: () => void;
};

/** A text filter input with its "Use regex" switch. */
export const TextPatternFields: React.FC<TextPatternFieldsProps> = ({
	text,
	isRegex,
	error,
	onTextChange,
	onRegexChange,
	onSubmit,
}) => {
	const regexId = useId();
	const hintId = useId();
	return (
		<>
			<Input
				aria-label="Text to match"
				value={text}
				placeholder={isRegex ? "Regex..." : "Text to match..."}
				spellCheck={false}
				autoComplete="off"
				aria-invalid={Boolean(error)}
				aria-describedby={hintId}
				className={cn("h-8", isRegex && "font-mono")}
				onChange={(event) => onTextChange(event.target.value)}
				onKeyDown={(event) => {
					if (event.key === "Enter" && !error) {
						event.preventDefault();
						onSubmit();
					}
				}}
			/>
			<SwitchRow id={regexId} label="Use regex">
				<Switch
					id={regexId}
					size="sm"
					checked={isRegex}
					onCheckedChange={onRegexChange}
				/>
			</SwitchRow>
			<FieldHint id={hintId} error={error}>
				{isRegex
					? "Uses RE2 syntax. Leave empty to match everything."
					: "Matches anywhere, ignoring case. Leave empty to match everything."}
			</FieldHint>
		</>
	);
};

type SwitchRowProps = {
	readonly id: string;
	readonly label: string;
	readonly children: React.ReactNode;
};

export const SwitchRow: React.FC<SwitchRowProps> = ({
	id,
	label,
	children,
}) => (
	<div className="flex items-center justify-between gap-3">
		<Label htmlFor={id} className="font-normal">
			{label}
		</Label>
		{children}
	</div>
);

type FieldHintProps = {
	readonly id: string;
	/** Replaces the hint while the field is invalid. */
	readonly error?: string;
	readonly children: React.ReactNode;
};

/** The note under a field, or its error. */
export const FieldHint: React.FC<FieldHintProps> = ({
	id,
	error,
	children,
}) => (
	<p
		id={id}
		className={cn(
			"m-0 text-xs",
			error ? "text-content-destructive" : "text-content-secondary",
		)}
	>
		{error ?? children}
	</p>
);

type SaveErrorProps = {
	readonly message: string | undefined;
};

/** Why the kept draft in an editor was not saved last time. */
export const SaveError: React.FC<SaveErrorProps> = ({ message }) =>
	message ? (
		<p role="alert" className="m-0 text-xs text-content-destructive">
			{message}
		</p>
	) : null;

type TriggerSecretFieldsProps = {
	readonly trigger: ChatProjectTrigger;
	readonly secret: TriggerSecretInfo;
	readonly onUpdate: TriggerChipProps["onUpdate"];
};

/** Focuses after the next render, for elements a state change mounts. */
const focusAfterRender = (ref: React.RefObject<HTMLElement | null>) =>
	requestAnimationFrame(() => ref.current?.focus());

type SecretValues = { readonly secret: string };

/**
 * Adds, replaces, or removes a Slack signing secret or GitHub webhook
 * secret. The secret is write-only: the server only reports whether one is
 * set.
 */
export const TriggerSecretFields: React.FC<TriggerSecretFieldsProps> = ({
	trigger,
	secret,
	onUpdate,
}) => {
	const inputId = useId();
	const hintId = useId();
	const inputRef = useRef<HTMLInputElement>(null);
	const replaceRef = useRef<HTMLButtonElement>(null);
	const [replacing, setReplacing] = useState(false);
	const [saveError, setSaveError] = useState<string>();
	// Saves `value`, or removes the secret when it is empty. Either view
	// replaces the other, so focus moves to the one that appears.
	const saveSecret = (
		value: string,
		helpers: Pick<FormikHelpers<SecretValues>, "setSubmitting" | "resetForm">,
	) => {
		setSaveError(undefined);
		onUpdate(secret.toRequest(value), {
			onSuccess: () => {
				helpers.setSubmitting(false);
				helpers.resetForm();
				setReplacing(false);
				focusAfterRender(value ? replaceRef : inputRef);
			},
			onError: (message) => {
				helpers.setSubmitting(false);
				setSaveError(message);
				focusAfterRender(value ? inputRef : replaceRef);
			},
		});
	};
	const form = useFormik<SecretValues>({
		initialValues: { secret: "" },
		// Submitting needs a non-empty secret; Remove saves the empty one.
		onSubmit: (values, helpers) => saveSecret(values.secret.trim(), helpers),
	});

	if (secret.isSet(trigger) && !replacing) {
		return (
			<div className="flex flex-col gap-2">
				<SaveError message={saveError} />
				<p className="m-0 flex items-center gap-1.5 text-sm font-medium text-content-primary">
					<ShieldCheckIcon
						aria-hidden
						className="size-icon-sm text-content-success"
					/>
					{secret.label} set
				</p>
				<p className="m-0 text-xs text-content-secondary">
					{secret.verifiedHint}
				</p>
				<div className="flex gap-2">
					<Button
						ref={replaceRef}
						size="sm"
						variant="outline"
						disabled={form.isSubmitting}
						onClick={() => {
							setReplacing(true);
							focusAfterRender(inputRef);
						}}
					>
						Replace
					</Button>
					<Button
						size="sm"
						variant="outline"
						disabled={form.isSubmitting}
						onClick={() => {
							form.setSubmitting(true);
							saveSecret("", form);
						}}
					>
						Remove
					</Button>
				</div>
			</div>
		);
	}

	const canSave = form.values.secret.trim() !== "" && !form.isSubmitting;
	return (
		<form
			className="flex flex-col gap-2"
			onSubmit={(event) => {
				event.preventDefault();
				if (canSave) {
					form.handleSubmit();
				}
			}}
		>
			<SaveError message={saveError} />
			<Label htmlFor={inputId}>{secret.label}</Label>
			<Input
				ref={inputRef}
				id={inputId}
				name="secret"
				type="password"
				value={form.values.secret}
				maxLength={triggerSecretMaxLength}
				autoComplete="off"
				spellCheck={false}
				aria-describedby={hintId}
				className="h-8 bg-surface-primary font-mono"
				onChange={form.handleChange}
			/>
			<FieldHint id={hintId}>{secret.hint}</FieldHint>
			<div className="flex justify-end gap-2">
				{replacing && (
					<Button
						type="button"
						size="sm"
						variant="outline"
						disabled={form.isSubmitting}
						onClick={() => {
							setReplacing(false);
							form.resetForm();
							setSaveError(undefined);
							focusAfterRender(replaceRef);
						}}
					>
						Cancel
					</Button>
				)}
				<Button type="submit" size="sm" disabled={!canSave}>
					Save secret
				</Button>
			</div>
		</form>
	);
};

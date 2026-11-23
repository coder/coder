import { useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { dynamicParameters } from "#/api/queries/templates";
import type { PreviewParameter } from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { RetryButton } from "#/components/RetryButton/RetryButton";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useDebouncedValue } from "#/hooks/debounce";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	Diagnostics,
	DynamicParameter,
} from "#/modules/workspaces/DynamicParameter/DynamicParameter";

type ParameterValues = Readonly<Record<string, string>>;

type ChatProjectDefaultParametersProps = {
	readonly templateVersionId: string;
	/** Values by parameter name. A missing name uses the template default. */
	readonly value: ParameterValues;
	readonly onChange: (value: ParameterValues) => void;
	readonly disabled: boolean;
};

/**
 * Edits a project's default values for a template version's parameters. A
 * value whose parameter the evaluation leaves out is kept, because another
 * value can bring that parameter back.
 */
export const ChatProjectDefaultParameters: React.FC<
	ChatProjectDefaultParametersProps
> = ({ templateVersionId, value, onChange, disabled }) => {
	// The evaluation needs a workspace owner. Chats in the project belong to
	// different users, so the form previews the parameters as its own user.
	const { user } = useAuthenticated();
	const groupRef = useRef<HTMLDivElement>(null);
	// Each evaluation is a request of its own, so typing and dragging wait
	// for a pause before the next one. Every other control evaluates at once.
	const [evaluationDelayMs, setEvaluationDelayMs] = useState(0);
	const inputs = useDebouncedValue(value, evaluationDelayMs);
	const evaluation = useQuery({
		...dynamicParameters(templateVersionId, user.id, inputs),
		placeholderData: keepPreviousData,
	});

	if (!evaluation.data && !evaluation.error) {
		return <Skeleton className="h-10 w-full" />;
	}

	const diagnostics = evaluation.data?.diagnostics ?? [];
	// The server sends null rather than [] when the evaluation fails as a
	// whole, which its diagnostics explain.
	const parameters = (evaluation.data?.parameters ?? []).toSorted(
		(a, b) => a.order - b.order,
	);

	return (
		<div
			ref={groupRef}
			role="group"
			aria-label="Parameter values"
			tabIndex={-1}
			className="flex flex-col gap-4 outline-hidden"
		>
			{/* Shown with cached parameters too, since a failed refetch means they
			    may be out of date. */}
			{evaluation.error && (
				<Alert
					severity="error"
					actions={
						<RetryButton
							size="sm"
							variant="outline"
							isRetrying={evaluation.isFetching}
							onRetry={() => {
								void evaluation.refetch();
								// Retry goes away while the parameters load again.
								groupRef.current?.focus();
							}}
						/>
					}
				>
					<AlertDescription>
						{getErrorMessage(evaluation.error, "Failed to load parameters.")}
					</AlertDescription>
				</Alert>
			)}
			{diagnostics.length > 0 && <Diagnostics diagnostics={diagnostics} />}
			{evaluation.data &&
				parameters.length === 0 &&
				diagnostics.length === 0 && (
					<p className="m-0 text-sm text-content-secondary">
						This template has no parameters.
					</p>
				)}
			{parameters.length > 0 && (
				<p className="m-0 text-xs text-content-secondary">
					Values you change are saved as the project's defaults. The other
					parameters keep the template's default.
				</p>
			)}
			{parameters.map((parameter) => (
				<ChatProjectDefaultParameter
					key={parameter.name}
					parameter={parameter}
					value={value[parameter.name]}
					onChange={(next) => {
						setEvaluationDelayMs(isAdjustedGradually(parameter) ? 500 : 0);
						onChange({ ...value, [parameter.name]: next });
					}}
					onReset={() => {
						setEvaluationDelayMs(0);
						onChange(
							Object.fromEntries(
								Object.entries(value).filter(
									([name]) => name !== parameter.name,
								),
							),
						);
					}}
					disabled={disabled}
				/>
			))}
		</div>
	);
};

const isAdjustedGradually = (parameter: PreviewParameter) =>
	parameter.form_type === "input" ||
	parameter.form_type === "textarea" ||
	parameter.form_type === "slider";

type ChatProjectDefaultParameterProps = {
	readonly parameter: PreviewParameter;
	/** Undefined when the parameter uses the template default. */
	readonly value: string | undefined;
	readonly onChange: (value: string) => void;
	/** Returns the parameter to the template default. */
	readonly onReset: () => void;
	readonly disabled: boolean;
};

const ChatProjectDefaultParameter: React.FC<
	ChatProjectDefaultParameterProps
> = ({ parameter, value, onChange, onReset, disabled }) => {
	const groupRef = useRef<HTMLFieldSetElement>(null);
	const name = parameter.display_name || parameter.name;

	return (
		// A fieldset, because `DynamicParameter` does not pass `disabled` on
		// to its tag input.
		<fieldset
			ref={groupRef}
			aria-label={name}
			tabIndex={-1}
			disabled={disabled}
			className="flex min-w-0 flex-col gap-1 outline-hidden"
		>
			<DynamicParameter
				parameter={parameter}
				value={
					value ??
					(parameter.default_value.valid ? parameter.default_value.value : "")
				}
				onChange={onChange}
				disabled={disabled}
			/>
			{value !== undefined && (
				<Button
					type="button"
					variant="subtle"
					size="sm"
					className="self-start"
					aria-label={`Use template default for ${name}`}
					disabled={disabled}
					onClick={() => {
						onReset();
						// The button goes away, so focus stays with its parameter.
						groupRef.current?.focus();
					}}
				>
					Use template default
				</Button>
			)}
		</fieldset>
	);
};

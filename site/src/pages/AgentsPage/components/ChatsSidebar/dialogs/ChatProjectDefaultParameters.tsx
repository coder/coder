import { useRef, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { dynamicParameters } from "#/api/queries/templates";
import type { PreviewParameter } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
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
 * Edits a project's default values for a template version's parameters. The
 * version is evaluated with the values set so far, as the workspace creation
 * form does, so parameters that depend on other values stay current.
 *
 * A value whose parameter the evaluation leaves out is kept, because another
 * value can bring that parameter back. Workspace builds ignore such values.
 */
export const ChatProjectDefaultParameters: React.FC<
	ChatProjectDefaultParametersProps
> = ({ templateVersionId, value, onChange, disabled }) => {
	// The evaluation needs a workspace owner. Chats in the project belong to
	// different users, so the form previews the parameters as its own user.
	const { user } = useAuthenticated();
	const groupRef = useRef<HTMLDivElement>(null);
	// Typing and dragging wait for a pause before the version is evaluated
	// again. Every other control evaluates at once.
	const [evaluationDelayMs, setEvaluationDelayMs] = useState(0);
	const inputs = useDebouncedValue(value, evaluationDelayMs);
	const evaluation = useQuery({
		...dynamicParameters(templateVersionId, user.id, inputs),
		placeholderData: keepPreviousData,
	});

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
			{evaluation.data ? (
				<>
					{diagnostics.length > 0 && <Diagnostics diagnostics={diagnostics} />}
					{parameters.length > 0 ? (
						<p className="m-0 text-xs text-content-secondary">
							Values you change are saved as the project's defaults. The other
							parameters keep the template's default.
						</p>
					) : (
						diagnostics.length === 0 && (
							<p className="m-0 text-sm text-content-secondary">
								This template has no parameters.
							</p>
						)
					)}
					{parameters.map((parameter) => (
						<ChatProjectDefaultParameter
							key={parameter.name}
							parameter={parameter}
							value={value[parameter.name]}
							onChange={(next) => {
								setEvaluationDelayMs(
									next !== undefined && isAdjustedGradually(parameter)
										? 500
										: 0,
								);
								const others = Object.fromEntries(
									Object.entries(value).filter(
										([name]) => name !== parameter.name,
									),
								);
								onChange(
									next === undefined
										? others
										: { ...others, [parameter.name]: next },
								);
							}}
							disabled={disabled}
						/>
					))}
				</>
			) : evaluation.error ? (
				// A failed evaluation hides the fields but keeps their values, so
				// trying again is the way back to them.
				<div className="flex flex-col items-start gap-2">
					<p role="alert" className="m-0 text-sm text-content-destructive">
						{getErrorMessage(evaluation.error, "Failed to load parameters.")}
					</p>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={disabled}
						onClick={() => {
							void evaluation.refetch();
							// The button goes away once the parameters load.
							groupRef.current?.focus();
						}}
					>
						Retry
					</Button>
				</div>
			) : (
				<Skeleton className="h-10 w-full" />
			)}
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
	readonly onChange: (value: string | undefined) => void;
	readonly disabled: boolean;
};

const ChatProjectDefaultParameter: React.FC<
	ChatProjectDefaultParameterProps
> = ({ parameter, value, onChange, disabled }) => {
	const groupRef = useRef<HTMLDivElement>(null);
	const name = parameter.display_name || parameter.name;

	return (
		<div
			ref={groupRef}
			role="group"
			aria-label={name}
			tabIndex={-1}
			className="flex flex-col gap-1 outline-hidden"
		>
			<DynamicParameter
				parameter={parameter}
				value={value ?? (parameter.value.valid ? parameter.value.value : "")}
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
						onChange(undefined);
						// The button goes away, so focus stays with its parameter.
						groupRef.current?.focus();
					}}
				>
					Use template default
				</Button>
			)}
		</div>
	);
};

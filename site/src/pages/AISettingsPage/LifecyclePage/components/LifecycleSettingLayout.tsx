import type { FC, FormEventHandler, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { TemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { cn } from "#/utils/cn";

interface LifecycleSettingLayoutProps {
	title: string;
	description: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	switchLabel: string;
	disabled?: boolean;
	children?: ReactNode;
	error?: ReactNode;
	showSave: boolean;
	isSaving: boolean;
	isSavedVisible: boolean;
	saveDisabled: boolean;
	onSubmit: FormEventHandler<HTMLFormElement>;
}

export const LifecycleSettingLayout: FC<LifecycleSettingLayoutProps> = ({
	title,
	description,
	checked,
	onCheckedChange,
	switchLabel,
	disabled,
	children,
	error,
	showSave,
	isSaving,
	isSavedVisible,
	saveDisabled,
	onSubmit,
}) => {
	return (
		<form className="flex items-start gap-3" onSubmit={onSubmit} noValidate>
			<Switch
				checked={checked}
				onCheckedChange={onCheckedChange}
				aria-label={switchLabel}
				disabled={disabled}
				className="mt-0.5"
			/>
			<div className="flex max-w-[980px] flex-1 flex-col">
				<h3 className="m-0 text-sm font-normal leading-6 text-content-primary">
					{title}
				</h3>
				<p className="mt-1 mb-0 text-sm font-normal leading-6 text-content-secondary">
					{description}
				</p>
				<div className="mt-4 flex flex-wrap items-start gap-3">
					{children}
					<div className="flex min-h-10 items-center">
						{(showSave || isSavedVisible || isSaving) &&
							(isSavedVisible ? (
								<TemporarySavedState />
							) : (
								<Button
									size="lg"
									type="submit"
									disabled={saveDisabled}
									className="h-10 min-w-[88px]"
								>
									{isSaving && <Spinner loading className="size-4" />}
									Save
								</Button>
							))}
					</div>
				</div>
				{error && (
					<div className="text-xs text-content-destructive">{error}</div>
				)}
			</div>
		</form>
	);
};

interface DaysFieldProps {
	name: string;
	value: number;
	onChange: (event: React.ChangeEvent<HTMLInputElement>) => void;
	onBlur: (event: React.FocusEvent<HTMLInputElement>) => void;
	label: string;
	disabled?: boolean;
	error?: boolean;
	min: number;
	max: number;
}

export const DaysField: FC<DaysFieldProps> = ({
	name,
	value,
	onChange,
	onBlur,
	label,
	disabled,
	error,
	min,
	max,
}) => {
	return (
		<label
			className={cn(
				"grid h-10 w-24 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-md border border-border-default border-solid bg-transparent px-3 transition-colors",
				error && "border-border-destructive",
				disabled && "opacity-50",
			)}
		>
			<span className="sr-only">{label}</span>
			<input
				type="number"
				name={name}
				min={min}
				max={max}
				step={1}
				aria-label={label}
				value={value}
				onChange={onChange}
				onBlur={onBlur}
				aria-invalid={error}
				disabled={disabled}
				className="min-w-0 w-full border-none bg-transparent p-0 text-sm font-medium leading-6 text-content-placeholder outline-none disabled:cursor-not-allowed [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none [-moz-appearance:textfield]"
			/>
			<span className="shrink-0 text-xs font-normal leading-[18px] text-content-placeholder">
				Days
			</span>
		</label>
	);
};

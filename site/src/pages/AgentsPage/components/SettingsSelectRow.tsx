import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

/**
 * One choice of a {@link SettingsSelectRow}. Its `value` is a member of the
 * row's string union `T`, which the row's `value` and `onValueChange` share.
 */
export type SettingsSelectOption<T extends string> = {
	value: T;
	label: string;
};

type SettingsSelectRowProps<T extends string> = {
	title: string;
	description: string;
	ariaLabel: string;
	value: T | undefined;
	placeholder?: string;
	disabled: boolean;
	options: readonly SettingsSelectOption<T>[];
	onValueChange: (value: T) => void;
	alert?: React.ReactNode;
	errorMessage: string | undefined;
};

/**
 * A titled preference row with a select. `onValueChange` fires only for a
 * value that one of `options` declares, so callers never receive an untyped
 * string from the underlying Select.
 */
export const SettingsSelectRow = <T extends string>({
	title,
	description,
	ariaLabel,
	value,
	placeholder,
	disabled,
	options,
	onValueChange,
	alert,
	errorMessage,
}: SettingsSelectRowProps<T>) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">{title}</h3>
		{alert}
		<div className="flex items-center justify-between gap-4">
			<p className="m-0 flex-1 text-xs text-content-secondary">{description}</p>
			<Select
				value={value ?? ""}
				disabled={disabled}
				onValueChange={(selectedValue: string) => {
					const selected = options.find((opt) => opt.value === selectedValue);
					if (selected) onValueChange(selected.value);
				}}
			>
				<SelectTrigger className="w-44 shrink-0" aria-label={ariaLabel}>
					<SelectValue placeholder={placeholder} />
				</SelectTrigger>
				<SelectContent>
					{options.map((opt) => (
						<SelectItem key={opt.value} value={opt.value}>
							{opt.label}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
		{errorMessage !== undefined && (
			<p role="alert" className="m-0 text-xs text-content-destructive">
				{errorMessage}
			</p>
		)}
	</div>
);

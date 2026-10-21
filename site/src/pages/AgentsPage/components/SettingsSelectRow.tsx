import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";

/** `label` is the visible text; `value` is what `onValueChange` receives. */
export type SettingsSelectOption<T extends string> = {
	value: T;
	label: string;
};

type SettingsSelectRowProps<T extends string> = {
	title: string;
	description: string;
	ariaLabel: string;
	value: T;
	disabled: boolean;
	options: readonly SettingsSelectOption<T>[];
	onValueChange: (value: T) => void;
	errorMessage: string | undefined;
};

/**
 * A titled preference row with a select. `errorMessage` renders as an alert
 * below the select unless it is undefined.
 */
export const SettingsSelectRow = <T extends string>({
	title,
	description,
	ariaLabel,
	value,
	disabled,
	options,
	onValueChange,
	errorMessage,
}: SettingsSelectRowProps<T>) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">{title}</h3>
		<div className="flex items-center justify-between gap-4">
			<p className="m-0 flex-1 text-xs text-content-secondary">{description}</p>
			<Select
				value={value}
				disabled={disabled}
				onValueChange={(selectedValue: string) => {
					const selected = options.find((opt) => opt.value === selectedValue);
					if (selected) onValueChange(selected.value);
				}}
			>
				<SelectTrigger className="w-44 shrink-0" aria-label={ariaLabel}>
					<SelectValue />
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

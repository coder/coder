import { cn } from "cn";
import { useId } from "react";

export type RadioCardOption<T extends string> = {
	value: T;
	label: string;
	description?: string;
	/** Controls shown inside the card, such as a picker for this choice. */
	children?: React.ReactNode;
};

type RadioCardsProps<T extends string> = {
	"aria-labelledby": string;
	"aria-describedby"?: string;
	value: T;
	options: readonly RadioCardOption<T>[];
	disabled?: boolean;
	onValueChange: (value: T) => void;
	className?: string;
};

/**
 * Side-by-side radio cards. They use native radios, not Radix RadioGroup:
 * Radix sends Shift+Tab from any other control inside the group back to the
 * checked radio, and a card here can hold a picker.
 */
export const RadioCards = <T extends string>({
	"aria-labelledby": labelledBy,
	"aria-describedby": describedBy,
	value,
	options,
	disabled = false,
	onValueChange,
	className,
}: RadioCardsProps<T>) => {
	const name = useId();
	return (
		<div
			role="radiogroup"
			aria-labelledby={labelledBy}
			aria-describedby={describedBy}
			aria-disabled={disabled || undefined}
			className={cn(
				"grid grid-cols-1 items-start gap-3 sm:grid-cols-2",
				className,
			)}
		>
			{options.map((option) => (
				<RadioCard
					key={option.value}
					name={name}
					option={option}
					checked={option.value === value}
					disabled={disabled}
					onSelect={() => onValueChange(option.value)}
				/>
			))}
		</div>
	);
};

type RadioCardProps<T extends string> = {
	name: string;
	option: RadioCardOption<T>;
	checked: boolean;
	disabled: boolean;
	onSelect: () => void;
};

const RadioCard = <T extends string>({
	name,
	option,
	checked,
	disabled,
	onSelect,
}: RadioCardProps<T>) => {
	const id = useId();
	const descriptionId = useId();
	return (
		<div
			className={cn(
				"flex min-w-0 flex-col gap-3 rounded-lg border border-solid p-4 transition-colors",
				checked ? "border-border-secondary" : "border-border",
			)}
		>
			<div className="flex items-start gap-2.5">
				<RadioInput
					id={id}
					name={name}
					value={option.value}
					checked={checked}
					disabled={disabled}
					aria-describedby={option.description ? descriptionId : undefined}
					onChange={onSelect}
				/>
				<div className="flex min-w-0 flex-col gap-0.5">
					<label
						htmlFor={id}
						className={cn(
							"text-sm text-content-primary",
							disabled ? "cursor-not-allowed" : "cursor-pointer",
						)}
					>
						{option.label}
					</label>
					{option.description && (
						<span id={descriptionId} className="text-xs text-content-secondary">
							{option.description}
						</span>
					)}
				</div>
			</div>
			{option.children}
		</div>
	);
};

/** A native radio drawn like the Radix RadioGroupItem. */
export const RadioInput: React.FC<React.ComponentProps<"input">> = ({
	className,
	...props
}) => (
	<input
		type="radio"
		className={cn(
			"m-0 mt-0.5 size-4 shrink-0 cursor-pointer appearance-none rounded-full border border-solid border-border bg-surface-primary",
			"checked:border-[5px] checked:border-surface-invert-primary",
			"focus:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link focus-visible:ring-offset-2 focus-visible:ring-offset-surface-primary",
			"disabled:cursor-not-allowed disabled:opacity-60",
			className,
		)}
		{...props}
	/>
);

type SegmentedRadiosProps<T extends string> = {
	"aria-labelledby": string;
	"aria-describedby"?: string;
	value: T;
	options: readonly { value: T; label: string }[];
	onValueChange: (value: T) => void;
};

/** A compact choice drawn as tabs, such as the trigger kind of a new automation. */
export const SegmentedRadios = <T extends string>({
	"aria-labelledby": labelledBy,
	"aria-describedby": describedBy,
	value,
	options,
	onValueChange,
}: SegmentedRadiosProps<T>) => {
	const name = useId();
	return (
		<div
			role="radiogroup"
			aria-labelledby={labelledBy}
			aria-describedby={describedBy}
			className="flex items-end"
		>
			{options.map((option) => {
				const checked = option.value === value;
				return (
					<label
						key={option.value}
						className={cn(
							"cursor-pointer border-0 border-b-2 border-solid px-3 pb-2 text-sm transition-colors",
							"has-[:focus-visible]:rounded-sm has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-content-link",
							checked
								? "border-content-primary text-content-primary"
								: "border-transparent text-content-secondary hover:text-content-primary",
						)}
					>
						<input
							type="radio"
							name={name}
							value={option.value}
							checked={checked}
							onChange={() => onValueChange(option.value)}
							className="sr-only"
						/>
						{option.label}
					</label>
				);
			})}
		</div>
	);
};

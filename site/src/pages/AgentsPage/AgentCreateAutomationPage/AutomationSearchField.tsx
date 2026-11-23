import { ChevronDownIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { FormField } from "#/components/FormField/FormField";
import { Spinner } from "#/components/Spinner/Spinner";
import type { FormHelpers } from "#/utils/formUtils";

type AutomationSearchFieldProps = {
	field: FormHelpers;
	label: string;
	value: string;
	onChange: (value: string) => void;
	options: readonly { value: string; label: string }[];
	placeholder: string;
	emptyMessage: string;
	disabled: boolean;
	loading?: boolean;
	noneLabel?: string;
	hasMore?: boolean;
	loadingMore?: boolean;
	onLoadMore?: () => void;
};

/** Required selections remain selected when picked again; None clears optional ones. */
export const AutomationSearchField: React.FC<AutomationSearchFieldProps> = ({
	field,
	label,
	value,
	onChange,
	options,
	placeholder,
	emptyMessage,
	disabled,
	loading,
	noneLabel,
	hasMore,
	loadingMore,
	onLoadMore,
}: AutomationSearchFieldProps) => {
	const selectedLabel = value
		? (options.find((option) => option.value === value)?.label ??
			"Selected option unavailable")
		: (noneLabel ?? placeholder);

	return (
		<FormField
			field={field}
			label={label}
			control={({
				id: controlId,
				"aria-invalid": invalid,
				"aria-describedby": describedBy,
			}) => (
				<Combobox
					value={value}
					onValueChange={(next) => {
						if (next && next !== "__none__") onChange(next);
					}}
				>
					<ComboboxTrigger asChild>
						<Button
							id={controlId}
							aria-invalid={invalid}
							aria-describedby={describedBy}
							type="button"
							variant="outline"
							className="w-full justify-between truncate"
							disabled={disabled || (loading && options.length === 0)}
						>
							<span className="truncate">
								{loading && options.length === 0
									? `Loading ${label.toLowerCase()}...`
									: selectedLabel}
							</span>
							<ChevronDownIcon aria-hidden className="size-4 shrink-0" />
						</Button>
					</ComboboxTrigger>
					<ComboboxContent
						aria-label={`${label} options`}
						label={`Search ${label.toLowerCase()}`}
						className="w-72 max-w-[calc(100vw-2rem)] p-0"
					>
						<ComboboxInput
							placeholder={`Search ${label.toLowerCase()}...`}
							disabled={disabled}
						/>
						<ComboboxList className="max-h-64 overflow-y-auto">
							{noneLabel && (
								<ComboboxItem
									value="__none__"
									onSelect={() => onChange("")}
									disabled={disabled}
								>
									{noneLabel}
								</ComboboxItem>
							)}
							{options.map((option) => (
								<ComboboxItem
									key={option.value}
									value={option.value}
									keywords={[option.label]}
									disabled={disabled}
								>
									<span className="truncate">{option.label}</span>
								</ComboboxItem>
							))}
						</ComboboxList>
						<ComboboxEmpty>{emptyMessage}</ComboboxEmpty>
						{hasMore && onLoadMore && (
							<div className="border-t p-2">
								<Button
									type="button"
									variant="outline"
									className="w-full"
									onClick={onLoadMore}
									onKeyDown={(event) => {
										// cmdk treats Enter as option selection unless propagation stops.
										if (event.key === "Enter") event.stopPropagation();
									}}
									disabled={disabled || loadingMore}
								>
									<Spinner loading={loadingMore} />
									Load more chats
								</Button>
							</div>
						)}
					</ComboboxContent>
				</Combobox>
			)}
		/>
	);
};

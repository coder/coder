import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "components/Combobox/Combobox";
import { Spinner } from "components/Spinner/Spinner";
import type { FC, ReactNode } from "react";
import { cn } from "utils/cn";

const BASE_WIDTH = 200;

export type SelectFilterOption = {
	startIcon?: ReactNode;
	label: string;
	value: string;
};

type SelectFilterProps = {
	options: SelectFilterOption[] | undefined;
	// For single-select, pass the currently selected option.
	selectedOption?: SelectFilterOption;
	// For multi-select, pass a Set of selected values.
	value?: Set<string>;
	label: string;
	placeholder: string;
	emptyText?: string;
	onSelect: (option: SelectFilterOption | undefined) => void;
	width?: number;
	selectFilterSearch?: ReactNode;
};

export const SelectFilter: FC<SelectFilterProps> = ({
	label,
	options,
	selectedOption,
	value,
	onSelect,
	placeholder,
	emptyText = "No options found",
	width = BASE_WIDTH,
	selectFilterSearch,
}) => {
	const isMultiple = value instanceof Set;
	const comboboxValue = isMultiple ? value : selectedOption?.value;

	const displayOption = isMultiple
		? value.size > 1
			? { label: `${value.size} selected`, value: "" }
			: value.size === 1
				? options?.find((o) => value.has(o.value))
				: undefined
		: selectedOption;

	return (
		<Combobox
			value={comboboxValue}
			onValueChange={(v) => onSelect(options?.find((opt) => opt.value === v))}
		>
			<ComboboxTrigger asChild>
				<ComboboxButton
					selectedOption={displayOption}
					placeholder={placeholder}
					className="flex-shrink-0 grow"
					style={{ flexBasis: width }}
					aria-label={label}
				/>
			</ComboboxTrigger>
			<ComboboxContent
				className={cn([selectFilterSearch && "w-full", "max-w-[260px]"])}
				style={{
					minWidth: width,
				}}
				align="end"
			>
				{selectFilterSearch}
				<ComboboxList
					className={cn(
						!selectFilterSearch && "border-t-0",
						"border-surface-quaternary",
					)}
				>
					{options !== undefined ? (
						options.map((option) => (
							<ComboboxItem
								className="px-4 data-[selected=true]:bg-surface-tertiary font-normal gap-4"
								key={option.value}
								value={option.value}
								keywords={[option.label]}
							>
								{option.startIcon}
								<span className="flex-1 truncate">{option.label}</span>
							</ComboboxItem>
						))
					) : (
						<div className="flex items-center justify-center py-4">
							<Spinner size="sm" loading />
						</div>
					)}
				</ComboboxList>
				{options !== undefined && <ComboboxEmpty>{emptyText}</ComboboxEmpty>}
			</ComboboxContent>
		</Combobox>
	);
};

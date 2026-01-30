import {
	Combobox,
	ComboboxButton,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "components/Combobox/Combobox";
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
	selectedOption?: SelectFilterOption;
	// Used to add a accessibility label to the select
	label: string;
	// Used when there is no option selected
	placeholder: string;
	// Used to customize the empty state message
	emptyText?: string;
	onSelect: (option: SelectFilterOption | undefined) => void;
	width?: number;
	// SelectFilterSearch element
	selectFilterSearch?: ReactNode;
};

export const SelectFilter: FC<SelectFilterProps> = ({
	label,
	options,
	selectedOption,
	onSelect,
	placeholder,
	emptyText,
	width = BASE_WIDTH,
	selectFilterSearch,
}) => {
	return (
		<Combobox
			value={selectedOption?.value}
			onValueChange={(value) =>
				onSelect(options?.find((opt) => opt.value === value))
			}
		>
			<ComboboxTrigger asChild>
				<ComboboxButton
					selectedOption={selectedOption}
					placeholder={placeholder}
					className="flex-shrink-0 grow"
					style={{ flexBasis: width }}
					aria-label={label}
				/>
			</ComboboxTrigger>
			<ComboboxContent
				className={cn([
					// When including selectFilterSearch, we aim for the width to be as
					// wide as possible.
					selectFilterSearch && "w-full",
					"max-w-[260px]",
				])}
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
					{options?.map((option) => (
						<ComboboxItem
							className="px-4 data-[selected=true]:bg-surface-tertiary font-normal gap-4"
							key={option.value}
							value={option.value}
						>
							{option.startIcon}
							<span className="flex-1 truncate">{option.label}</span>
						</ComboboxItem>
					))}
				</ComboboxList>
				<ComboboxEmpty>{emptyText}</ComboboxEmpty>
			</ComboboxContent>
		</Combobox>
	);
};

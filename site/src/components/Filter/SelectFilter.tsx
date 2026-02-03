import { Loader } from "components/Loader/Loader";
import type { SearchFieldProps } from "components/SearchField/SearchField";
import {
	SelectMenu,
	SelectMenuButton,
	SelectMenuContent,
	SelectMenuIcon,
	SelectMenuItem,
	SelectMenuList,
	SelectMenuSearch,
	SelectMenuTrigger,
} from "components/SelectMenu/SelectMenu";
import { type FC, type ReactNode, useState } from "react";
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
	// SelectFilterSearch element
	selectFilterSearch?: ReactNode;
	width?: number;
};

export const SelectFilter: FC<SelectFilterProps> = ({
	label,
	options,
	selectedOption,
	onSelect,
	placeholder,
	emptyText,
	selectFilterSearch,
	width = BASE_WIDTH,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<SelectMenu open={open} onOpenChange={setOpen}>
			<SelectMenuTrigger>
				<SelectMenuButton
					startIcon={selectedOption?.startIcon}
					className="shrink-0 grow"
					style={{ flexBasis: width }}
					aria-label={label}
				>
					{selectedOption?.label ?? placeholder}
				</SelectMenuButton>
			</SelectMenuTrigger>
			<SelectMenuContent
				align="end"
				className={cn([
					// When including selectFilterSearch, we aim for the width to be as
					// wide as possible.
					selectFilterSearch && "w-full",
					"max-w-[320px]",
				])}
				style={{
					minWidth: width,
				}}
			>
				{selectFilterSearch}
				{options ? (
					options.length > 0 ? (
						<SelectMenuList>
							{options.map((o) => {
								const isSelected = o.value === selectedOption?.value;
								return (
									<SelectMenuItem
										key={o.value}
										selected={isSelected}
										onClick={() => {
											setOpen(false);
											onSelect(isSelected ? undefined : o);
										}}
									>
										{o.startIcon && (
											<SelectMenuIcon>{o.startIcon}</SelectMenuIcon>
										)}
										{o.label}
									</SelectMenuItem>
								);
							})}
						</SelectMenuList>
					) : (
						<div
							css={(theme) => ({
								display: "flex",
								alignItems: "center",
								justifyContent: "center",
								padding: 32,
								color: theme.palette.text.secondary,
								lineHeight: 1,
							})}
						>
							{emptyText || "No options found"}
						</div>
					)
				) : (
					<Loader size="sm" />
				)}
			</SelectMenuContent>
		</SelectMenu>
	);
};

export const SelectFilterSearch = ({
	className,
	...props
}: SearchFieldProps) => {
	return (
		<SelectMenuSearch
			className={cn(
				className,
				"rounded-none border-x-0 border-t-0",
				"has-[input:focus-visible]:ring-0",
			)}
			{...props}
		/>
	);
};

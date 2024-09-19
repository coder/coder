import { Loader } from "components/Loader/Loader";
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

const BASE_WIDTH = 200;
const POPOVER_WIDTH = 320;

export type SelectFilterOption = {
	startIcon?: ReactNode;
	label: string;
	value: string;
};

export type SelectFilterProps = {
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
					css={{ flexBasis: width, flexGrow: 1 }}
					aria-label={label}
				>
					{selectedOption?.label ?? placeholder}
				</SelectMenuButton>
			</SelectMenuTrigger>
			<SelectMenuContent
				horizontal="right"
				css={{
					"& .MuiPaper-root": {
						// When including selectFilterSearch, we aim for the width to be as
						// wide as possible.
						width: selectFilterSearch ? "100%" : undefined,
						maxWidth: POPOVER_WIDTH,
						minWidth: width,
					},
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
					<Loader size={16} />
				)}
			</SelectMenuContent>
		</SelectMenu>
	);
};

export const SelectFilterSearch = SelectMenuSearch;

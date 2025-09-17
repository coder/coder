import { SearchIcon } from "lucide-react";
import type { FC, HTMLAttributes, InputHTMLAttributes, Ref } from "react";
import { cn } from "utils/cn";

interface SearchProps extends HTMLAttributes<HTMLDivElement> {
	ref?: Ref<HTMLDivElement>;
}

/**
 * A container component meant for `SearchInput`
 *
 * ```
 * <Search>
 *   <SearchInput />
 * </Search>
 * ```
 */
export const Search: FC<SearchProps> = ({
	children,
	ref,
	className,
	...props
}) => {
	return (
		<div
			ref={ref}
			{...props}
			className={cn(
				"flex items-center h-10 pl-4 border-0 border-b border-solid border-border",
				className,
			)}
		>
			<SearchIcon className="size-icon-xs text-sm text-content-secondary" />
			{children}
		</div>
	);
};

type SearchInputProps = InputHTMLAttributes<HTMLInputElement> & {
	label?: string;
	ref?: Ref<HTMLInputElement>;
};

export const SearchInput: FC<SearchInputProps> = ({
	label,
	ref,
	id,
	...inputProps
}) => {
	return (
		<>
			<label className="sr-only" htmlFor={id}>
				{label}
			</label>
			<input
				ref={ref}
				id={id}
				tabIndex={0}
				type="text"
				placeholder="Search..."
				className="text-inherit h-full border-0 bg-transparent grow basis-0 outline-none pl-4 placeholder:text-content-secondary"
				{...inputProps}
			/>
		</>
	);
};

export const SearchEmpty: FC<HTMLAttributes<HTMLDivElement>> = ({
	children = "Not found",
	...props
}) => {
	return (
		<div className="text-sm text-content-secondary text-center py-2" {...props}>
			{children}
		</div>
	);
};

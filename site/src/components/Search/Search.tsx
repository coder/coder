// biome-ignore lint/style/noRestrictedImports: use it to have the component prop
import type { Interpolation } from "@emotion/react";
import type { Theme } from "@mui/material";
import { SearchIcon } from "lucide-react";
import {
	type FC,
	type HTMLAttributes,
	type InputHTMLAttributes,
	type Ref,
	useId,
} from "react";
import { cn } from "utils/cn";

interface SearchProps extends Omit<HTMLAttributes<HTMLDivElement>, "ref"> {
	$$ref?: Ref<HTMLDivElement>;
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
	$$ref,
	className,
	...boxProps
}) => {
	return (
		<div
			ref={$$ref}
			{...boxProps}
			className={cn(
				"flex items-center pl-4 h-10 border-b border-solid",
				className,
			)}
		>
			<SearchIcon className="size-icon-xs text-content-secondary" />
			{children}
		</div>
	);
};

type SearchInputProps = InputHTMLAttributes<HTMLInputElement> & {
	label?: string;
	$$ref?: Ref<HTMLInputElement>;
};

export const SearchInput: FC<SearchInputProps> = ({
	$$ref,
	id,
	label,
	className,
	...inputProps
}) => {
	const hookId = useId();
	const inputId = id || `${hookId}-input`;

	return (
		<>
			<label className="sr-only" htmlFor={inputId}>
				{label}
			</label>
			<input
				ref={$$ref}
				id={inputId}
				tabIndex={-1}
				type="text"
				placeholder="Search..."
				{...inputProps}
				className={cn(
					"text-inherit h-full border-none bg-transparent grow shrink ml-4 outline-none placeholder:text-content-secondary",
					className,
				)}
			/>
		</>
	);
};

export const SearchEmpty: FC<HTMLAttributes<HTMLDivElement>> = ({
	children = "Not found",
	className,
	...props
}) => {
	return (
		<div
			{...props}
			className={cn(
				"text-[13px] text-content-secondary items-center py-2",
				className,
			)}
		>
			{children}
		</div>
	);
};

/**
 * Reusable styles for consumers of the base components
 */
export const searchStyles = {
	content: {
		width: 320,
		padding: 0,
		borderRadius: 4,
	},
} satisfies Record<string, Interpolation<Theme>>;

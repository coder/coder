import type { Interpolation, Theme } from "@emotion/react";
// biome-ignore lint/nursery/noRestrictedImports: use it to have the component prop
import Box, { type BoxProps } from "@mui/material/Box";
import visuallyHidden from "@mui/utils/visuallyHidden";
import type { FC, HTMLAttributes, InputHTMLAttributes, Ref } from "react";

interface SearchProps extends Omit<BoxProps, "ref"> {
	$$ref?: Ref<unknown>;
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
export const Search: FC<SearchProps> = ({ children, $$ref, ...boxProps }) => {
	return (
		<Box ref={$$ref} {...boxProps} css={SearchStyles.container}>
			<SearchOutlined css={SearchStyles.icon} />
			{children}
		</Box>
	);
};

const SearchStyles = {
	container: (theme) => ({
		display: "flex",
		alignItems: "center",
		paddingLeft: 16,
		height: 40,
		borderBottom: `1px solid ${theme.palette.divider}`,
	}),

	icon: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

type SearchInputProps = InputHTMLAttributes<HTMLInputElement> & {
	label?: string;
	$$ref?: Ref<HTMLInputElement>;
};

export const SearchInput: FC<SearchInputProps> = ({
	label,
	$$ref,
	...inputProps
}) => {
	return (
		<>
			<label css={{ ...visuallyHidden }} htmlFor={inputProps.id}>
				{label}
			</label>
			<input
				ref={$$ref}
				tabIndex={-1}
				type="text"
				placeholder="Search..."
				css={SearchInputStyles.input}
				{...inputProps}
			/>
		</>
	);
};

const SearchInputStyles = {
	input: (theme) => ({
		color: "inherit",
		height: "100%",
		border: 0,
		background: "none",
		flex: 1,
		marginLeft: 16,
		outline: 0,
		"&::placeholder": {
			color: theme.palette.text.secondary,
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;

export const SearchEmpty: FC<HTMLAttributes<HTMLDivElement>> = ({
	children = "Not found",
	...props
}) => {
	return (
		<div css={SearchEmptyStyles.empty} {...props}>
			{children}
		</div>
	);
};

const SearchEmptyStyles = {
	empty: (theme) => ({
		fontSize: 13,
		color: theme.palette.text.secondary,
		textAlign: "center",
		paddingTop: 8,
		paddingBottom: 8,
	}),
} satisfies Record<string, Interpolation<Theme>>;

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

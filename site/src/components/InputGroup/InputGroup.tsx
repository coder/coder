import type { FC, HTMLProps } from "react";

export const InputGroup: FC<HTMLProps<HTMLDivElement>> = (props) => {
	return (
		<div
			{...props}
			css={{
				display: "flex",
				alignItems: "flex-start",

				// Overlap borders to avoid displaying double borders between elements.
				"& > *:not(:last-child)": {
					marginRight: -1,
				},

				// Ensure the border of the hovered element is visible when borders
				// overlap.
				"& > *:hover": {
					zIndex: 1,
				},

				// Display border elements when focused or in an error state, both of
				// which take priority over hover.
				"& .Mui-focused, & .Mui-error": {
					zIndex: 2,
				},

				"& > *:first-of-type": {
					borderTopRightRadius: 0,
					borderBottomRightRadius: 0,
				},

				"& > *:last-child": {
					borderTopLeftRadius: 0,
					borderBottomLeftRadius: 0,

					"&.MuiFormControl-root .MuiInputBase-root": {
						borderTopLeftRadius: 0,
						borderBottomLeftRadius: 0,
					},
				},

				"& > *:not(:first-of-type):not(:last-child)": {
					borderRadius: 0,

					"&.MuiFormControl-root .MuiInputBase-root": {
						borderRadius: 0,
					},
				},
			}}
		/>
	);
};

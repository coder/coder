import Button, { type ButtonProps } from "@mui/material/Button";
import { forwardRef } from "react";

export const AgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
	(props, ref) => {
		const { children, ...buttonProps } = props;

		return (
			<Button
				{...buttonProps}
				color="neutral"
				size="xlarge"
				variant="contained"
				ref={ref}
				css={(theme) => ({
					padding: "12px 20px",
					color: theme.palette.text.primary,
					// Making them smaller since those icons don't have a padding around them
					"& .MuiButton-startIcon, & .MuiButton-endIcon": {
						width: 16,
						height: 16,

						"& svg, & img": { width: "100%", height: "100%" },
					},
				})}
			>
				{children}
			</Button>
		);
	},
);

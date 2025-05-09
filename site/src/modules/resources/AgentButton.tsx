import { Button, type ButtonProps } from "components/Button/Button";
import { forwardRef } from "react";

export const AgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
	(props, ref) => {
		return <Button variant="outline" ref={ref} {...props} />;
	},
);

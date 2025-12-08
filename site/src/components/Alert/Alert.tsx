import MuiAlert, {
	type AlertColor as MuiAlertColor,
	type AlertProps as MuiAlertProps,
	// biome-ignore lint/style/noRestrictedImports: Used as base component
} from "@mui/material/Alert";
import Collapse from "@mui/material/Collapse";
import { Button } from "components/Button/Button";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useState,
} from "react";
export type AlertColor = MuiAlertColor;

export type AlertProps = MuiAlertProps & {
	actions?: ReactNode;
	dismissible?: boolean;
	onDismiss?: () => void;
};

export const Alert: FC<AlertProps> = ({
	children,
	actions,
	dismissible,
	severity = "info",
	onDismiss,
	...alertProps
}) => {
	const [open, setOpen] = useState(true);

	// Can't only rely on MUI's hiding behavior inside flex layouts, because even
	// though MUI will make a dismissed alert have zero height, the alert will
	// still behave as a flex child and introduce extra row/column gaps
	if (!open) {
		return null;
	}

	return (
		<Collapse in>
			<MuiAlert
				{...alertProps}
				className="text-left"
				severity={severity}
				action={
					<>
						{/* CTAs passed in by the consumer */}
						{actions}

						{/* close CTA */}
						{dismissible && (
							<Button
								variant="subtle"
								size="sm"
								onClick={() => {
									setOpen(false);
									onDismiss?.();
								}}
								data-testid="dismiss-banner-btn"
							>
								Dismiss
							</Button>
						)}
					</>
				}
			>
				{children}
			</MuiAlert>
		</Collapse>
	);
};

export const AlertDetail: FC<PropsWithChildren> = ({ children }) => {
	return (
		<span className={"text-sm text-content-secondary"} data-chromatic="ignore">
			{children}
		</span>
	);
};

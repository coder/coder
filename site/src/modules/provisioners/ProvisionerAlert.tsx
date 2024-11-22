import { Alert, AlertColor, AlertTitle } from "@mui/material";
import { AlertDetail } from "components/Alert/Alert";
import { type FC } from "react";

type ProvisionerAlertProps = {
	title: string,
	detail: string,
	severity: AlertColor,
}

export const ProvisionerAlert : FC<ProvisionerAlertProps> = ({
	title,
	detail,
	severity,
}) => {
	return (
		<Alert
			severity={severity}
			css={(theme) => ({
				borderRadius: 0,
				border: 0,
				borderBottom: `1px solid ${theme.palette.divider}`,
				borderLeft: `2px solid ${theme.palette.error.main}`,
			})}
		>
			<AlertTitle>{title}</AlertTitle>
			<AlertDetail>{detail}</AlertDetail>
		</Alert>
	);
};

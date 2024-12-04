import AlertTitle from "@mui/material/AlertTitle";
import { Alert, type AlertColor } from "components/Alert/Alert";
import { AlertDetail } from "components/Alert/Alert";
import { Stack } from "components/Stack/Stack";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import type { FC } from "react";

export enum AlertVariant {
	Standalone = "Standalone",
	InLogs = "InLogs",
}

interface ProvisionerAlertProps {
	title: string;
	detail: string;
	severity: AlertColor;
	tags: Record<string, string>;
	variant?: AlertVariant;
}

export const ProvisionerAlert: FC<ProvisionerAlertProps> = ({
	title,
	detail,
	severity,
	tags,
	variant = AlertVariant.Standalone,
}) => {
	return (
		<Alert
			severity={severity}
			{...(variant === AlertVariant.InLogs && {
				css: (theme) => ({
					borderRadius: 0,
					border: 0,
					borderBottom: `1px solid ${theme.palette.divider}`,
					borderLeft: `2px solid ${theme.palette[severity].main}`,
				}),
			})}
		>
			<AlertTitle>{title}</AlertTitle>
			<AlertDetail>
				<div>{detail}</div>
				<Stack direction="row" spacing={1} wrap="wrap">
					{Object.entries(tags ?? {})
						.filter(([key]) => key !== "owner")
						.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</Stack>
			</AlertDetail>
		</Alert>
	);
};

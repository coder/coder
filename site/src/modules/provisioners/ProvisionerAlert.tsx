import AlertTitle from "@mui/material/AlertTitle";
import { Alert, type AlertColor } from "components/Alert/Alert";
import { AlertDetail } from "components/Alert/Alert";
import { Stack } from "components/Stack/Stack";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import type { FC } from "react";

interface ProvisionerAlertProps {
	matchingProvisioners: number | undefined;
	availableProvisioners: number | undefined;
	tags: Record<string, string>;
}

export const ProvisionerAlert: FC<ProvisionerAlertProps> = ({
	matchingProvisioners,
	availableProvisioners,
	tags,
}) => {
	let title: string;
	let detail: string;
	switch (true) {
		case matchingProvisioners === 0:
			title = "Provisioning Cannot Proceed";
			detail =
				"There are no provisioners that accept the required tags. Please contact your administrator. Once a compatible provisioner becomes available, provisioning will continue.";
			break;
		case availableProvisioners === 0:
			title = "Provisioning Delayed";
			detail =
				"Provisioners that accept the required tags are currently anavailable. This may delay your build. Please contact your administrator if your build does not complete.";
			break;
		default:
			return null;
	}

	return (
		<Alert
			severity="warning"
			css={(theme) => ({
				borderRadius: 0,
				border: 0,
				borderBottom: `1px solid ${theme.palette.divider}`,
				borderLeft: `2px solid ${theme.palette.error.main}`,
			})}
		>
			<AlertTitle>{title}</AlertTitle>
			<AlertDetail>
				<div>{detail}</div>
				<Stack direction="row" spacing={1} wrap="wrap">
					{Object.entries(tags)
						.filter(([key]) => key !== "owner")
						.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</Stack>
			</AlertDetail>
		</Alert>
	);
};

interface ProvisionerJobAlertProps {
	title: string;
	detail: string;
	severity: AlertColor;
	tags: Record<string, string>;
}

export const ProvisionerJobAlert: FC<ProvisionerJobAlertProps> = ({
	title,
	detail,
	severity,
	tags,
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
			<AlertDetail>
				<div>{detail}</div>
				<Stack direction="row" spacing={1} wrap="wrap">
					{Object.entries(tags)
						.filter(([key]) => key !== "owner")
						.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</Stack>
			</AlertDetail>
		</Alert>
	);
};

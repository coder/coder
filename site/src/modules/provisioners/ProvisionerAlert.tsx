import type { Theme } from "@emotion/react";
import AlertTitle from "@mui/material/AlertTitle";
import { Alert, type AlertColor } from "components/Alert/Alert";
import { AlertDetail } from "components/Alert/Alert";
import { Stack } from "components/Stack/Stack";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import type { FC } from "react";

export enum AlertVariant {
	// Alerts are usually styled with a full rounded border and meant to use as a visually distinct element of the page.
	// The Standalone variant conforms to this styling.
	Standalone = "Standalone",
	// We show these same alerts in environments such as log drawers where we stream the logs from builds.
	// In this case the full border is incongruent with the surroundings of the component.
	// The Inline variant replaces the full rounded border with a left border and a divider so that it complements the surroundings.
	Inline = "Inline",
}

interface ProvisionerAlertProps {
	title: string;
	detail: string;
	severity: AlertColor;
	tags: Record<string, string>;
	variant?: AlertVariant;
}

const getAlertStyles = (variant: AlertVariant, severity: AlertColor) => {
	switch (variant) {
		case AlertVariant.Inline:
			return {
				css: (theme: Theme) => ({
					borderRadius: 0,
					border: 0,
					borderBottom: `1px solid ${theme.palette.divider}`,
					borderLeft: `2px solid ${theme.palette[severity].main}`,
				}),
			};
		default:
			return {};
	}
};

export const ProvisionerAlert: FC<ProvisionerAlertProps> = ({
	title,
	detail,
	severity,
	tags,
	variant = AlertVariant.Standalone,
}) => {
	return (
		<Alert severity={severity} {...getAlertStyles(variant, severity)}>
			<AlertTitle>{title}</AlertTitle>
			<AlertDetail>
				<div>{detail}</div>
				<div className="flex items-center gap-2 flex-wrap mt-2">
					{Object.entries(tags ?? {})
						.filter(([key]) => key !== "owner")
						.map(([key, value]) => (
							<ProvisionerTag key={key} tagName={key} tagValue={value} />
						))}
				</div>
			</AlertDetail>
		</Alert>
	);
};

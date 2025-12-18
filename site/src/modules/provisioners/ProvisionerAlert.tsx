import {
	Alert,
	type AlertColor,
	AlertDetail,
	AlertTitle,
} from "components/Alert/Alert";
import { ProvisionerTag } from "modules/provisioners/ProvisionerTag";
import type { FC } from "react";
import { cn } from "utils/cn";

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

const severityBorderColors: Record<AlertColor, string> = {
	info: "border-l-highlight-sky",
	success: "border-l-content-success",
	warning: "border-l-content-warning",
	error: "border-l-content-destructive",
};

const getAlertClassName = (variant: AlertVariant, severity: AlertColor) => {
	if (variant === AlertVariant.Inline) {
		return cn(
			"rounded-none border-0 border-b border-l-2 border-solid border-b-border-default",
			severityBorderColors[severity],
		);
	}
	return undefined;
};

export const ProvisionerAlert: FC<ProvisionerAlertProps> = ({
	title,
	detail,
	severity,
	tags,
	variant = AlertVariant.Standalone,
}) => {
	return (
		<Alert severity={severity} className={getAlertClassName(variant, severity)}>
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

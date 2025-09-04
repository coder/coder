import type { Theme } from "@emotion/react";
import { Alert, AlertDetail, AlertTitle, type AlertProps } from "components/Alert/Alert";
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
	variant: NonNullable<AlertProps["variant"]>;
	tags: Record<string, string>;
	alertVariant?: AlertVariant;
}

const getAlertStyles = (alertVariant: AlertVariant, variant: NonNullable<AlertProps["variant"]>) => {
	switch (alertVariant) {
		case AlertVariant.Inline:
			return {
				css: (theme: Theme) => ({
					borderRadius: 0,
					border: 0,
					borderBottom: `1px solid ${theme.palette.divider}`,
					borderLeft: `3px solid ${
						variant === "destructive"
							? theme.palette.error.main
							: variant === "warning"
								? theme.palette.warning.main
								: variant === "success"
									? theme.palette.success.main
									: theme.palette.info.main
					}`,
				}),
			};
		default:
			return {};
	}
};

export const ProvisionerAlert: FC<ProvisionerAlertProps> = ({
	title,
	detail,
	variant,
	tags,
	alertVariant = AlertVariant.Standalone,
}) => {
	return (
		<Alert variant={variant} {...getAlertStyles(alertVariant, variant)}>
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
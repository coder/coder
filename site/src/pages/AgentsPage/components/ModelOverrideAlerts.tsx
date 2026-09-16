import type { FC, ReactNode } from "react";
import { Alert, AlertDescription } from "#/components/Alert/Alert";

interface ModelOverrideAlertsProps {
	isUnavailableSavedModel: boolean;
	unavailableMessage: ReactNode;
	modelsError: unknown;
	children?: ReactNode;
}

export const ModelOverrideAlerts: FC<ModelOverrideAlertsProps> = ({
	isUnavailableSavedModel,
	unavailableMessage,
	modelsError,
	children,
}) => {
	return (
		<>
			{isUnavailableSavedModel && (
				<Alert severity="warning">
					<AlertDescription>{unavailableMessage}</AlertDescription>
				</Alert>
			)}
			{children}
			{Boolean(modelsError) && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load models.
				</p>
			)}
		</>
	);
};

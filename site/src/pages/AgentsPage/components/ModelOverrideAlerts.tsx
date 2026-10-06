import { Alert, AlertDescription } from "#/components/Alert/Alert";

type ModelOverrideAlertsProps = {
	isUnavailableSavedModel: boolean;
	unavailableMessage: React.ReactNode;
	modelsError: unknown;
	children?: React.ReactNode;
};

export const ModelOverrideAlerts: React.FC<ModelOverrideAlertsProps> = ({
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

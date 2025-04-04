import CircularProgress from "@mui/material/CircularProgress";
import { updateAppearanceSettings } from "api/queries/users";
import { appearanceSettings } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Section } from "../Section";
import { AppearanceForm } from "./AppearanceForm";

export const AppearancePage: FC = () => {
	const queryClient = useQueryClient();
	const updateAppearanceSettingsMutation = useMutation(
		updateAppearanceSettings(queryClient),
	);

	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);

	if (appearanceSettingsQuery.isLoading) {
		return <Loader />;
	}

	if (!appearanceSettingsQuery.data) {
		return <ErrorAlert error={appearanceSettingsQuery.error} />;
	}

	return (
		<>
			<Section
				title={
					<Stack direction="row" alignItems="center">
						<span>Theme</span>
						{updateAppearanceSettingsMutation.isLoading && (
							<CircularProgress size={16} />
						)}
					</Stack>
				}
				layout="fluid"
			>
				<AppearanceForm
					isUpdating={updateAppearanceSettingsMutation.isLoading}
					error={updateAppearanceSettingsMutation.error}
					initialValues={{
						theme_preference: appearanceSettingsQuery.data.theme_preference,
						terminal_font: appearanceSettingsQuery.data.terminal_font,
					}}
					onSubmit={updateAppearanceSettingsMutation.mutateAsync}
				/>
			</Section>
		</>
	);
};

export default AppearancePage;

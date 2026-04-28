import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	appearanceSettings,
	updateAppearanceSettings,
} from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";
import { AppearanceForm } from "./AppearanceForm";

const AppearancePage: FC = () => {
	const queryClient = useQueryClient();
	const updateAppearanceSettingsMutation = useMutation(
		updateAppearanceSettings(queryClient),
	);

	const { metadata } = useEmbeddedMetadata();
	const appearanceSettingsQuery = useQuery(
		appearanceSettings(metadata.userAppearance),
	);
	const osColorScheme = usePreferredColorScheme();

	if (appearanceSettingsQuery.isLoading) {
		return <Loader />;
	}

	if (!appearanceSettingsQuery.data) {
		return <ErrorAlert error={appearanceSettingsQuery.error} />;
	}

	return (
		<AppearanceForm
			isUpdating={updateAppearanceSettingsMutation.isPending}
			error={updateAppearanceSettingsMutation.error}
			initialValues={appearanceSettingsQuery.data}
			activeScheme={osColorScheme}
			onSubmit={updateAppearanceSettingsMutation.mutateAsync}
		/>
	);
};

export default AppearancePage;

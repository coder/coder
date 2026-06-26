import { type FC, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	appearanceSettings,
	updateAppearanceSettings,
} from "#/api/queries/users";
import type { UpdateUserAppearanceSettingsRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { usePreferredColorScheme } from "#/theme/usePreferredColorScheme";
import { AppearanceForm } from "./AppearanceForm";

type MutateAppearanceSettings = (
	values: UpdateUserAppearanceSettingsRequest,
	options: { onSettled: () => void },
) => void;

export const useQueuedAppearanceSubmit = (mutate: MutateAppearanceSettings) => {
	const submitInFlightRef = useRef(false);
	const pendingSubmitRef = useRef<UpdateUserAppearanceSettingsRequest | null>(
		null,
	);

	const fireSubmit = (values: UpdateUserAppearanceSettingsRequest) => {
		submitInFlightRef.current = true;
		mutate(values, {
			onSettled: () => {
				const queued = pendingSubmitRef.current;
				pendingSubmitRef.current = null;
				if (queued !== null) {
					fireSubmit(queued);
					return;
				}
				submitInFlightRef.current = false;
			},
		});
	};

	return (values: UpdateUserAppearanceSettingsRequest) => {
		if (submitInFlightRef.current) {
			pendingSubmitRef.current = values;
			return;
		}

		fireSubmit(values);
	};
};

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
	const submitAppearanceSettings = useQueuedAppearanceSubmit(
		(values, options) => {
			updateAppearanceSettingsMutation.mutate(values, {
				onSettled: options.onSettled,
			});
		},
	);

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
			onSubmit={submitAppearanceSettings}
		/>
	);
};

export default AppearancePage;

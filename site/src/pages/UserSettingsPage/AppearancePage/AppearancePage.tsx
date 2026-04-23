import { type FC, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	appearanceSettings,
	updateAppearanceSettings,
} from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { AppearanceForm } from "./AppearanceForm";

/**
 * Hook that tracks the user's OS color scheme. Mirrors the logic in
 * `ThemeProvider` but is scoped to the appearance page, which needs
 * the same signal to decide which sync card is Active.
 */
const useOsColorScheme = (): "dark" | "light" => {
	const mediaQuery = useMemo(
		() => window.matchMedia?.("(prefers-color-scheme: light)"),
		[],
	);
	const [scheme, setScheme] = useState<"dark" | "light">(
		mediaQuery?.matches ? "light" : "dark",
	);
	useEffect(() => {
		if (!mediaQuery) {
			return;
		}
		const listener = (event: MediaQueryListEvent) => {
			setScheme(event.matches ? "light" : "dark");
		};
		mediaQuery.addEventListener?.("change", listener);
		return () => mediaQuery.removeEventListener?.("change", listener);
	}, [mediaQuery]);
	return scheme;
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
	const osColorScheme = useOsColorScheme();

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

import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { refreshHealth } from "#/api/queries/debug";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useHealthStatus } from "#/pages/HealthPage/useHealthStatus";
import HealthSidebarView from "./HealthSidebarView";

/**
 * Health settings rail. Loads the shared healthcheck query and wires
 * the refresh mutation into the presentational sidebar view.
 */
export const HealthSidebar: FC = () => {
	const queryClient = useQueryClient();
	const { data: healthStatus, isLoading, error } = useHealthStatus();
	const { mutate: forceRefresh, isPending: isRefreshing } = useMutation(
		refreshHealth(queryClient),
	);

	if (isLoading) {
		return <Loader />;
	}

	if (error || !healthStatus) {
		return <ErrorAlert error={error} />;
	}

	return (
		<HealthSidebarView
			healthStatus={healthStatus}
			isRefreshing={isRefreshing}
			onRefresh={() => {
				forceRefresh();
			}}
		/>
	);
};

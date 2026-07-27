import { BellIcon, BellOffIcon } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { healthSettings, updateHealthSettings } from "#/api/queries/debug";
import type { HealthSection } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";

export const DismissWarningButton = (props: { healthcheck: HealthSection }) => {
	const queryClient = useQueryClient();
	const healthSettingsQuery = useQuery(healthSettings());
	// They call the same mutation but are used in diff contexts so we don't want
	// to merge their states. Eg. You mute a warning and when it is done it
	// will show the unmute button but since the mutation is still invalidating
	// other queries it will be in the loading state when it should be idle.
	const unmuteMutation = useMutation(updateHealthSettings(queryClient));
	const muteMutation = useMutation(updateHealthSettings(queryClient));

	if (!healthSettingsQuery.data) {
		return <Skeleton height={36} width={170} className="rounded-lg" />;
	}

	const { dismissed_healthchecks } = healthSettingsQuery.data;
	const isMuted = dismissed_healthchecks.includes(props.healthcheck);

	if (isMuted) {
		return (
			<Button
				disabled={healthSettingsQuery.isLoading || unmuteMutation.isPending}
				variant="outline"
				onClick={async () => {
					const updatedSettings = dismissed_healthchecks.filter(
						(dismissedHealthcheck) =>
							dismissedHealthcheck !== props.healthcheck,
					);
					await unmuteMutation.mutateAsync({
						dismissed_healthchecks: updatedSettings,
					});
					toast.success("Warnings unmuted successfully.");
				}}
			>
				<Spinner loading={unmuteMutation.isPending}>
					<BellOffIcon />
				</Spinner>
				Unmute warnings
			</Button>
		);
	}

	return (
		<Button
			disabled={healthSettingsQuery.isLoading || muteMutation.isPending}
			variant="outline"
			onClick={async () => {
				const updatedSettings = [...dismissed_healthchecks, props.healthcheck];
				await muteMutation.mutateAsync({
					dismissed_healthchecks: updatedSettings,
				});
				toast.success("Warnings muted successfully.");
			}}
		>
			<Spinner loading={muteMutation.isPending}>
				<BellIcon />
			</Spinner>
			Mute warnings
		</Button>
	);
};

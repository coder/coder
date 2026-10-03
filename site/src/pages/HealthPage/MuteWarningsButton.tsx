import { BellIcon, BellOffIcon } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { healthSettings, updateHealthSettings } from "#/api/queries/debug";
import type { HealthSection } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";

export const MuteWarningsButton = (props: { healthcheck: HealthSection }) => {
	const queryClient = useQueryClient();
	const healthSettingsQuery = useQuery(healthSettings());
	// Separate mutation instances so unmuting isn't stuck pending while
	// muting's query invalidation resolves (a shared mutation would share
	// isPending and spin the wrong button).
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

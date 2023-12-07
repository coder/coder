import NotificationOutlined from "@mui/icons-material/NotificationsOutlined";
import NotificationsOffOutlined from "@mui/icons-material/NotificationsOffOutlined";
import { healthSettings, updateHealthSettings } from "api/queries/debug";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import LoadingButton from "@mui/lab/LoadingButton";
import Skeleton from "@mui/material/Skeleton";
import { HealthSection } from "api/typesGenerated";

export const DismissWarningButton = (props: { healthcheck: HealthSection }) => {
  const queryClient = useQueryClient();
  const healthSettingsQuery = useQuery(healthSettings());
  // They call the same mutation but are used in diff contexts so we don't want
  // to merge their states. Eg. You dismiss a warning and when it is done it
  // will show the enable button but since the mutation is still invalidating
  // other queries it will be in the loading state when it should be idle.
  const enableMutation = useMutation(updateHealthSettings(queryClient));
  const dismissMutation = useMutation(updateHealthSettings(queryClient));

  if (!healthSettingsQuery.data) {
    return (
      <Skeleton
        variant="rectangular"
        height={36}
        width={170}
        css={{ borderRadius: 8 }}
      />
    );
  }

  const { dismissed_healthchecks } = healthSettingsQuery.data;
  const isDismissed = dismissed_healthchecks.includes(props.healthcheck);

  if (isDismissed) {
    return (
      <LoadingButton
        disabled={healthSettingsQuery.isLoading}
        loading={enableMutation.isLoading}
        loadingPosition="start"
        startIcon={<NotificationsOffOutlined />}
        onClick={async () => {
          const updatedSettings = dismissed_healthchecks.filter(
            (dismissedHealthcheck) =>
              dismissedHealthcheck !== props.healthcheck,
          );
          await enableMutation.mutateAsync({
            dismissed_healthchecks: updatedSettings,
          });
          displaySuccess("Warnings enabled successfully!");
        }}
      >
        Enable warnings
      </LoadingButton>
    );
  }

  return (
    <LoadingButton
      disabled={healthSettingsQuery.isLoading}
      loading={dismissMutation.isLoading}
      loadingPosition="start"
      startIcon={<NotificationOutlined />}
      onClick={async () => {
        const updatedSettings = [...dismissed_healthchecks, props.healthcheck];
        await dismissMutation.mutateAsync({
          dismissed_healthchecks: updatedSettings,
        });
        displaySuccess("Warnings dismissed successfully!");
      }}
    >
      Dismiss warnings
    </LoadingButton>
  );
};

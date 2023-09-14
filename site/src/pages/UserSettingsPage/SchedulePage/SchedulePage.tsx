import { FC } from "react";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Section } from "components/SettingsLayout/Section";
import { ScheduleForm } from "./ScheduleForm";
import { useMe } from "hooks/useMe";
import { Loader } from "components/Loader/Loader";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  updateUserQuietHoursSchedule,
  userQuietHoursSchedule,
} from "api/queries/settings";
import { displaySuccess } from "components/GlobalSnackbar/utils";

export const SchedulePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();

  const {
    data: quietHoursSchedule,
    error,
    isLoading,
    isError,
  } = useQuery(userQuietHoursSchedule(me.id));

  const updateSchedule = updateUserQuietHoursSchedule(me.id, queryClient);
  const {
    mutate: onSubmit,
    error: mutationError,
    isLoading: mutationLoading,
  } = useMutation({
    ...updateSchedule,
    onSuccess: async () => {
      await updateSchedule.onSuccess();
      displaySuccess("Schedule updated successfully");
    },
  });

  if (isLoading) {
    return <Loader />;
  }

  if (isError) {
    return <ErrorAlert error={error} />;
  }

  return (
    <Section
      title="Quiet hours"
      layout="fluid"
      description="Workspaces may be automatically updated during your quiet hours, as configured by your administrators."
    >
      <ScheduleForm
        isLoading={mutationLoading}
        initialValues={quietHoursSchedule}
        mutationError={mutationError}
        onSubmit={onSubmit}
      />
    </Section>
  );
};

export default SchedulePage;

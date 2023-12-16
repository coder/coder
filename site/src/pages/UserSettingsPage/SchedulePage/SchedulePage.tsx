import { type FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useMe } from "hooks/useMe";
import { Loader } from "components/Loader/Loader";
import {
  updateUserQuietHoursSchedule,
  userQuietHoursSchedule,
} from "api/queries/settings";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Section } from "../Section";
import { ScheduleForm } from "./ScheduleForm";

export const SchedulePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();

  const {
    data: quietHoursSchedule,
    error,
    isLoading,
    isError,
  } = useQuery(userQuietHoursSchedule(me.id));

  const {
    mutate: onSubmit,
    error: submitError,
    isLoading: mutationLoading,
  } = useMutation(updateUserQuietHoursSchedule(me.id, queryClient));

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
        submitError={submitError}
        onSubmit={(values) => {
          onSubmit(values, {
            onSuccess: () => {
              displaySuccess("Schedule updated successfully");
            },
          });
        }}
      />
    </Section>
  );
};

export default SchedulePage;

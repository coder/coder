import { FC, useState } from "react";
import { Section } from "../../../components/SettingsLayout/Section";
import { ScheduleForm } from "./ScheduleForm";
import { useMe } from "hooks/useMe";
import {
  UpdateUserQuietHoursScheduleRequest,
  UserQuietHoursScheduleResponse,
} from "api/typesGenerated";
import * as API from "api/api";
import { Loader } from "components/Loader/Loader";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  userQuietHoursSchedule,
  userQuietHoursScheduleKey,
} from "api/queries/settings";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export const SchedulePage: FC = () => {
  const me = useMe();
  const queryClient = useQueryClient();

  const [, setQuietHoursSchedule] = useState<
    UserQuietHoursScheduleResponse | undefined
  >(undefined);
  const [quietHoursSubmitting, setQuietHoursSubmitting] =
    useState<boolean>(false);
  const [quietHoursScheduleError, setQuietHoursScheduleError] =
    useState<unknown>("");

  const {
    data: quietHoursSchedule,
    error,
    isLoading,
    isError,
  } = useQuery(userQuietHoursSchedule(me.id));

  const onSubmit = async (data: UpdateUserQuietHoursScheduleRequest) => {
    setQuietHoursSubmitting(true);
    API.updateUserQuietHoursSchedule(me.id, data)
      .then((response) => {
        setQuietHoursSchedule(response);
        setQuietHoursSubmitting(false);
        setQuietHoursScheduleError(undefined);
        displaySuccess("Schedule updated successfully");
      })
      .catch((error) => {
        setQuietHoursSubmitting(false);
        setQuietHoursScheduleError(error);
      });
  };

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
        isLoading={quietHoursSubmitting}
        initialValues={quietHoursSchedule}
        refetch={async () => {
          queryClient.invalidateQueries(userQuietHoursScheduleKey(me.id));
        }}
        updateErr={quietHoursScheduleError}
        onSubmit={onSubmit}
      />
    </Section>
  );
};

export default SchedulePage;

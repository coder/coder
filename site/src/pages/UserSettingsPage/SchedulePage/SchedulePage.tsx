import { FC, useEffect, useState } from "react";
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

export const SchedulePage: FC = () => {
  const me = useMe();

  const [quietHoursSchedule, setQuietHoursSchedule] = useState<
    UserQuietHoursScheduleResponse | undefined
  >(undefined);
  const [quietHoursSubmitting, setQuietHoursSubmitting] =
    useState<boolean>(false);
  const [quietHoursScheduleError, setQuietHoursScheduleError] =
    useState<unknown>("");

  useEffect(() => {
    setQuietHoursSchedule(undefined);
    setQuietHoursScheduleError(undefined);
    API.getUserQuietHoursSchedule(me.id)
      .then((response) => {
        setQuietHoursSchedule(response);
        setQuietHoursScheduleError(undefined);
      })
      .catch((error) => {
        setQuietHoursSchedule(undefined);
        setQuietHoursScheduleError(error);
      });
  }, [me.id]);

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

  return (
    <Section title="Schedule" description="Manage your quiet hours schedule">
      {quietHoursSchedule === undefined && <Loader />}

      {quietHoursSchedule !== undefined && (
        <ScheduleForm
          submitting={quietHoursSubmitting}
          initialValues={quietHoursSchedule}
          updateErr={quietHoursScheduleError}
          onSubmit={onSubmit}
        />
      )}
    </Section>
  );
};

export default SchedulePage;

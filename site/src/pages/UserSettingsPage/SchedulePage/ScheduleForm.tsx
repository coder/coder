import TextField from "@mui/material/TextField";
import { FormikContextType, useFormik } from "formik";
import { FC, useEffect, useState } from "react";
import * as Yup from "yup";
import { getFormHelpers } from "utils/formUtils";
import { LoadingButton } from "components/LoadingButton/LoadingButton";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Form, FormFields } from "components/Form/Form";
import {
  UpdateUserQuietHoursScheduleRequest,
  UserQuietHoursScheduleResponse,
} from "api/typesGenerated";
import cronParser from "cron-parser";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import timezone from "dayjs/plugin/timezone";
import utc from "dayjs/plugin/utc";

dayjs.extend(timezone);
dayjs.extend(relativeTime);
dayjs.extend(utc);

export interface ScheduleFormValues {
  hours: number;
  minutes: number;
  timezone: string;
}

const validationSchema = Yup.object({
  hours: Yup.number().min(0).max(23).required(),
  minutes: Yup.number().min(0).max(59).required(),
  timezone: Yup.string().required(),
});

export interface ScheduleFormProps {
  submitting: boolean;
  initialValues: UserQuietHoursScheduleResponse;
  updateErr: unknown;
  onSubmit: (data: UpdateUserQuietHoursScheduleRequest) => void;
  // now can be set to force the time used for "Next occurrence" in tests.
  now?: Date;
}

export const ScheduleForm: FC<React.PropsWithChildren<ScheduleFormProps>> = ({
  submitting,
  initialValues,
  updateErr,
  onSubmit,
  now,
}) => {
  // Force a re-render every 15 seconds to update the "Next occurrence" field.
  // The app re-renders by itself occasionally but this is just to be sure it
  // doesn't get stale.
  const [_, setTime] = useState<number>(Date.now());
  useEffect(() => {
    const interval = setInterval(() => setTime(Date.now()), 15000);
    return () => {
      clearInterval(interval);
    };
  }, []);

  const preferredTimezone = getPreferredTimezone();
  const allTimezones = getAllTimezones();
  if (!allTimezones.includes(initialValues.timezone)) {
    allTimezones.push(initialValues.timezone);
  }
  if (!allTimezones.includes(preferredTimezone)) {
    allTimezones.push(preferredTimezone);
  }
  allTimezones.sort();

  // If the user has a custom schedule, use that as the initial values.
  // Otherwise, use midnight in their preferred timezone.
  const formInitialValues = {
    hours: 0,
    minutes: 0,
    timezone: preferredTimezone,
  };
  if (initialValues.user_set) {
    formInitialValues.hours = parseInt(initialValues.time.split(":")[0], 10);
    formInitialValues.minutes = parseInt(initialValues.time.split(":")[1], 10);
    formInitialValues.timezone = initialValues.timezone;
  }

  const form: FormikContextType<ScheduleFormValues> =
    useFormik<ScheduleFormValues>({
      initialValues: formInitialValues,
      validationSchema,
      onSubmit: (values) => {
        return onSubmit({
          schedule: timeToCron(values.hours, values.minutes, values.timezone),
        });
      },
    });
  const getFieldHelpers = getFormHelpers<ScheduleFormValues>(form, updateErr);

  return (
    <>
      <Form onSubmit={form.handleSubmit}>
        <FormFields>
          {Boolean(updateErr) && <ErrorAlert error={updateErr} />}

          <p>
            Workspaces will only be turned off for updates (due to settings{" "}
            configured by template admins) when your configured quiet hours{" "}
            start. Workspaces may still be automatically stopped at any other
            time due to inactivity.
          </p>

          {!initialValues.user_set && (
            <p>
              You are currently using the default quiet hours schedule, which is
              every day at <code>{initialValues.time}</code> in{" "}
              <code>{initialValues.timezone}</code>. You can set a custom quiet
              hours schedule below.
            </p>
          )}

          <Stack direction="row">
            <TextField
              {...getFieldHelpers("hours")}
              disabled={submitting}
              fullWidth
              inputProps={{ min: 0, max: 23, step: 1 }}
              label="Hours"
              type="number"
              helperText="24-hour time"
              style={{ marginRight: "1rem" }}
            />

            <TextField
              {...getFieldHelpers("minutes")}
              disabled={submitting}
              fullWidth
              inputProps={{ min: 0, max: 59, step: 1 }}
              label="Minutes"
              type="number"
            />
          </Stack>

          <TextField
            {...getFieldHelpers("timezone")}
            disabled={submitting}
            fullWidth
            label="Timezone"
            select
          >
            {allTimezones.map((tz) => (
              <MenuItem key={tz} value={tz}>
                {tz}
              </MenuItem>
            ))}
          </TextField>

          <TextField
            disabled
            fullWidth
            label="Cron schedule"
            value={timeToCron(
              form.values.hours,
              form.values.minutes,
              form.values.timezone,
            )}
          />

          <TextField
            disabled
            fullWidth
            label="Next occurrence"
            value={formatNextRun(
              form.values.hours,
              form.values.minutes,
              form.values.timezone,
              now,
            )}
          />

          <div>
            <LoadingButton
              loading={submitting}
              disabled={submitting}
              type="submit"
              variant="contained"
            >
              {submitting ? "" : "Update schedule"}
            </LoadingButton>
          </div>
        </FormFields>
      </Form>
    </>
  );
};

// Taken from https://stackoverflow.com/a/54500197
const allTimezonesFallback = [
  "Africa/Abidjan",
  "Africa/Accra",
  "Africa/Algiers",
  "Africa/Bissau",
  "Africa/Cairo",
  "Africa/Casablanca",
  "Africa/Ceuta",
  "Africa/El_Aaiun",
  "Africa/Johannesburg",
  "Africa/Juba",
  "Africa/Khartoum",
  "Africa/Lagos",
  "Africa/Maputo",
  "Africa/Monrovia",
  "Africa/Nairobi",
  "Africa/Ndjamena",
  "Africa/Sao_Tome",
  "Africa/Tripoli",
  "Africa/Tunis",
  "Africa/Windhoek",
  "America/Adak",
  "America/Anchorage",
  "America/Araguaina",
  "America/Argentina/Buenos_Aires",
  "America/Argentina/Catamarca",
  "America/Argentina/Cordoba",
  "America/Argentina/Jujuy",
  "America/Argentina/La_Rioja",
  "America/Argentina/Mendoza",
  "America/Argentina/Rio_Gallegos",
  "America/Argentina/Salta",
  "America/Argentina/San_Juan",
  "America/Argentina/San_Luis",
  "America/Argentina/Tucuman",
  "America/Argentina/Ushuaia",
  "America/Asuncion",
  "America/Atikokan",
  "America/Bahia",
  "America/Bahia_Banderas",
  "America/Barbados",
  "America/Belem",
  "America/Belize",
  "America/Blanc-Sablon",
  "America/Boa_Vista",
  "America/Bogota",
  "America/Boise",
  "America/Cambridge_Bay",
  "America/Campo_Grande",
  "America/Cancun",
  "America/Caracas",
  "America/Cayenne",
  "America/Chicago",
  "America/Chihuahua",
  "America/Costa_Rica",
  "America/Creston",
  "America/Cuiaba",
  "America/Curacao",
  "America/Danmarkshavn",
  "America/Dawson",
  "America/Dawson_Creek",
  "America/Denver",
  "America/Detroit",
  "America/Edmonton",
  "America/Eirunepe",
  "America/El_Salvador",
  "America/Fort_Nelson",
  "America/Fortaleza",
  "America/Glace_Bay",
  "America/Godthab",
  "America/Goose_Bay",
  "America/Grand_Turk",
  "America/Guatemala",
  "America/Guayaquil",
  "America/Guyana",
  "America/Halifax",
  "America/Havana",
  "America/Hermosillo",
  "America/Indiana/Indianapolis",
  "America/Indiana/Knox",
  "America/Indiana/Marengo",
  "America/Indiana/Petersburg",
  "America/Indiana/Tell_City",
  "America/Indiana/Vevay",
  "America/Indiana/Vincennes",
  "America/Indiana/Winamac",
  "America/Inuvik",
  "America/Iqaluit",
  "America/Jamaica",
  "America/Juneau",
  "America/Kentucky/Louisville",
  "America/Kentucky/Monticello",
  "America/La_Paz",
  "America/Lima",
  "America/Los_Angeles",
  "America/Maceio",
  "America/Managua",
  "America/Manaus",
  "America/Martinique",
  "America/Matamoros",
  "America/Mazatlan",
  "America/Menominee",
  "America/Merida",
  "America/Metlakatla",
  "America/Mexico_City",
  "America/Miquelon",
  "America/Moncton",
  "America/Monterrey",
  "America/Montevideo",
  "America/Nassau",
  "America/New_York",
  "America/Nipigon",
  "America/Nome",
  "America/Noronha",
  "America/North_Dakota/Beulah",
  "America/North_Dakota/Center",
  "America/North_Dakota/New_Salem",
  "America/Ojinaga",
  "America/Panama",
  "America/Pangnirtung",
  "America/Paramaribo",
  "America/Phoenix",
  "America/Port-au-Prince",
  "America/Port_of_Spain",
  "America/Porto_Velho",
  "America/Puerto_Rico",
  "America/Punta_Arenas",
  "America/Rainy_River",
  "America/Rankin_Inlet",
  "America/Recife",
  "America/Regina",
  "America/Resolute",
  "America/Rio_Branco",
  "America/Santarem",
  "America/Santiago",
  "America/Santo_Domingo",
  "America/Sao_Paulo",
  "America/Scoresbysund",
  "America/Sitka",
  "America/St_Johns",
  "America/Swift_Current",
  "America/Tegucigalpa",
  "America/Thule",
  "America/Thunder_Bay",
  "America/Tijuana",
  "America/Toronto",
  "America/Vancouver",
  "America/Whitehorse",
  "America/Winnipeg",
  "America/Yakutat",
  "America/Yellowknife",
  "Antarctica/Casey",
  "Antarctica/Davis",
  "Antarctica/Macquarie",
  "Antarctica/Mawson",
  "Antarctica/Palmer",
  "Antarctica/Rothera",
  "Antarctica/Syowa",
  "Antarctica/Troll",
  "Antarctica/Vostok",
  "Asia/Almaty",
  "Asia/Amman",
  "Asia/Anadyr",
  "Asia/Aqtau",
  "Asia/Aqtobe",
  "Asia/Ashgabat",
  "Asia/Atyrau",
  "Asia/Baghdad",
  "Asia/Baku",
  "Asia/Bangkok",
  "Asia/Barnaul",
  "Asia/Beirut",
  "Asia/Bishkek",
  "Asia/Brunei",
  "Asia/Chita",
  "Asia/Choibalsan",
  "Asia/Colombo",
  "Asia/Damascus",
  "Asia/Dhaka",
  "Asia/Dili",
  "Asia/Dubai",
  "Asia/Dushanbe",
  "Asia/Famagusta",
  "Asia/Gaza",
  "Asia/Hebron",
  "Asia/Ho_Chi_Minh",
  "Asia/Hong_Kong",
  "Asia/Hovd",
  "Asia/Irkutsk",
  "Asia/Jakarta",
  "Asia/Jayapura",
  "Asia/Jerusalem",
  "Asia/Kabul",
  "Asia/Kamchatka",
  "Asia/Karachi",
  "Asia/Kathmandu",
  "Asia/Khandyga",
  "Asia/Kolkata",
  "Asia/Krasnoyarsk",
  "Asia/Kuala_Lumpur",
  "Asia/Kuching",
  "Asia/Macau",
  "Asia/Magadan",
  "Asia/Makassar",
  "Asia/Manila",
  "Asia/Nicosia",
  "Asia/Novokuznetsk",
  "Asia/Novosibirsk",
  "Asia/Omsk",
  "Asia/Oral",
  "Asia/Pontianak",
  "Asia/Pyongyang",
  "Asia/Qatar",
  "Asia/Qyzylorda",
  "Asia/Riyadh",
  "Asia/Sakhalin",
  "Asia/Samarkand",
  "Asia/Seoul",
  "Asia/Shanghai",
  "Asia/Singapore",
  "Asia/Srednekolymsk",
  "Asia/Taipei",
  "Asia/Tashkent",
  "Asia/Tbilisi",
  "Asia/Tehran",
  "Asia/Thimphu",
  "Asia/Tokyo",
  "Asia/Tomsk",
  "Asia/Ulaanbaatar",
  "Asia/Urumqi",
  "Asia/Ust-Nera",
  "Asia/Vladivostok",
  "Asia/Yakutsk",
  "Asia/Yangon",
  "Asia/Yekaterinburg",
  "Asia/Yerevan",
  "Atlantic/Azores",
  "Atlantic/Bermuda",
  "Atlantic/Canary",
  "Atlantic/Cape_Verde",
  "Atlantic/Faroe",
  "Atlantic/Madeira",
  "Atlantic/Reykjavik",
  "Atlantic/South_Georgia",
  "Atlantic/Stanley",
  "Australia/Adelaide",
  "Australia/Brisbane",
  "Australia/Broken_Hill",
  "Australia/Currie",
  "Australia/Darwin",
  "Australia/Eucla",
  "Australia/Hobart",
  "Australia/Lindeman",
  "Australia/Lord_Howe",
  "Australia/Melbourne",
  "Australia/Perth",
  "Australia/Sydney",
  "Europe/Amsterdam",
  "Europe/Andorra",
  "Europe/Astrakhan",
  "Europe/Athens",
  "Europe/Belgrade",
  "Europe/Berlin",
  "Europe/Brussels",
  "Europe/Bucharest",
  "Europe/Budapest",
  "Europe/Chisinau",
  "Europe/Copenhagen",
  "Europe/Dublin",
  "Europe/Gibraltar",
  "Europe/Helsinki",
  "Europe/Istanbul",
  "Europe/Kaliningrad",
  "Europe/Kiev",
  "Europe/Kirov",
  "Europe/Lisbon",
  "Europe/London",
  "Europe/Luxembourg",
  "Europe/Madrid",
  "Europe/Malta",
  "Europe/Minsk",
  "Europe/Monaco",
  "Europe/Moscow",
  "Europe/Oslo",
  "Europe/Paris",
  "Europe/Prague",
  "Europe/Riga",
  "Europe/Rome",
  "Europe/Samara",
  "Europe/Saratov",
  "Europe/Simferopol",
  "Europe/Sofia",
  "Europe/Stockholm",
  "Europe/Tallinn",
  "Europe/Tirane",
  "Europe/Ulyanovsk",
  "Europe/Uzhgorod",
  "Europe/Vienna",
  "Europe/Vilnius",
  "Europe/Volgograd",
  "Europe/Warsaw",
  "Europe/Zaporozhye",
  "Europe/Zurich",
  "Indian/Chagos",
  "Indian/Christmas",
  "Indian/Cocos",
  "Indian/Kerguelen",
  "Indian/Mahe",
  "Indian/Maldives",
  "Indian/Mauritius",
  "Indian/Reunion",
  "Pacific/Apia",
  "Pacific/Auckland",
  "Pacific/Bougainville",
  "Pacific/Chatham",
  "Pacific/Chuuk",
  "Pacific/Easter",
  "Pacific/Efate",
  "Pacific/Enderbury",
  "Pacific/Fakaofo",
  "Pacific/Fiji",
  "Pacific/Funafuti",
  "Pacific/Galapagos",
  "Pacific/Gambier",
  "Pacific/Guadalcanal",
  "Pacific/Guam",
  "Pacific/Honolulu",
  "Pacific/Kiritimati",
  "Pacific/Kosrae",
  "Pacific/Kwajalein",
  "Pacific/Majuro",
  "Pacific/Marquesas",
  "Pacific/Nauru",
  "Pacific/Niue",
  "Pacific/Norfolk",
  "Pacific/Noumea",
  "Pacific/Pago_Pago",
  "Pacific/Palau",
  "Pacific/Pitcairn",
  "Pacific/Pohnpei",
  "Pacific/Port_Moresby",
  "Pacific/Rarotonga",
  "Pacific/Tahiti",
  "Pacific/Tarawa",
  "Pacific/Tongatapu",
  "Pacific/Wake",
  "Pacific/Wallis",
];

const getAllTimezones = () => {
  if (Intl.supportedValuesOf) {
    return Intl.supportedValuesOf("timeZone").sort();
  }

  return allTimezonesFallback;
};

const getPreferredTimezone = () => {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
};

const timeToCron = (hours: number, minutes: number, tz: string) => {
  let prefix = "";
  if (tz !== "") {
    prefix = `CRON_TZ=${tz} `;
  }
  return `${prefix}${minutes} ${hours} * * *`;
};

// evaluateNextRun returns a Date object of the next cron run time.
const evaluateNextRun = (
  hours: number,
  minutes: number,
  tz: string,
  now: Date | undefined,
): Date => {
  // The cron-parser package doesn't accept a timezone in the cron string, but
  // accepts it as an option.
  const cron = timeToCron(hours, minutes, "");
  const parsed = cronParser.parseExpression(cron, {
    currentDate: now,
    iterator: false,
    utc: false,
    tz: tz,
  });

  return parsed.next().toDate();
};

const formatNextRun = (
  hours: number,
  minutes: number,
  tz: string,
  now: Date | undefined,
): string => {
  const nowDjs = dayjs(now).tz(tz);
  const djs = dayjs(evaluateNextRun(hours, minutes, tz, now)).tz(tz);
  let str = djs.format("h:mm A");
  if (djs.isSame(nowDjs, "day")) {
    str += " today";
  } else if (djs.isSame(nowDjs.add(1, "day"), "day")) {
    str += " tomorrow";
  } else {
    // This case will rarely ever be hit, as we're dealing with only times and
    // not dates, but it can be hit due to mismatched browser timezone to cron
    // timezone or due to daylight savings changes.
    str += ` on ${djs.format("dddd, MMMM D")}`;
  }

  str += ` (${djs.from(now)})`;

  return str;
};

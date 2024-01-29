import Checkbox from "@mui/material/Checkbox";
import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import LoadingButton from "@mui/lab/LoadingButton";
import { type FormikContextType, useFormik } from "formik";
import { type FC } from "react";
import * as Yup from "yup";
import type * as TypesGen from "api/typesGenerated";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { docs } from "utils/docs";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { FormFields, VerticalForm } from "components/Form/Form";
import { CoderIcon } from "components/Icons/CoderIcon";
import MenuItem from "@mui/material/MenuItem";
import { countries } from "./countries";
import Autocomplete from "@mui/material/Autocomplete";
import { Stack } from "components/Stack/Stack";

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  usernameLabel: "Username",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordRequired: "Please enter a password.",
  create: "Create account",
  welcomeMessage: <>Welcome to Coder</>,

  firstNameLabel: "First name",
  lastNameLabel: "Last name",
  companyLabel: "Company",
  jobTitleLabel: "Job title",
  phoneNumberLabel: "Phone number",
  countryLabel: "Country",
  developersLabel: "Number of developers",
  firstNameRequired: "Please enter your first name.",
  phoneNumberRequired: "Please enter your phone number.",
  jobTitleRequired: "Please enter your job title.",
  companyNameRequired: "Please enter your company name.",
  countryRequired: "Please select your country.",
  developersRequired: "Please select the number of developers in your company.",
};

const validationSchema = Yup.object({
  email: Yup.string()
    .trim()
    .email(Language.emailInvalid)
    .required(Language.emailRequired),
  password: Yup.string().required(Language.passwordRequired),
  username: nameValidator(Language.usernameLabel),
  trial: Yup.bool(),
  trial_info: Yup.object().when("trial", {
    is: true,
    then: (schema) =>
      schema.shape({
        first_name: Yup.string().required(Language.firstNameRequired),
        last_name: Yup.string().required(Language.firstNameRequired),
        phone_number: Yup.string().required(Language.phoneNumberRequired),
        job_title: Yup.string().required(Language.jobTitleRequired),
        company_name: Yup.string().required(Language.companyNameRequired),
        country: Yup.string().required(Language.countryRequired),
        developers: Yup.string().required(Language.developersRequired),
      }),
  }),
});

const numberOfDevelopersOptions = [
  "1-100",
  "101-500",
  "501-1000",
  "1001-2500",
  "2500+",
];

export interface SetupPageViewProps {
  onSubmit: (firstUser: TypesGen.CreateFirstUserRequest) => void;
  error?: unknown;
  isLoading?: boolean;
}

export const SetupPageView: FC<SetupPageViewProps> = ({
  onSubmit,
  error,
  isLoading,
}) => {
  const form: FormikContextType<TypesGen.CreateFirstUserRequest> =
    useFormik<TypesGen.CreateFirstUserRequest>({
      initialValues: {
        email: "",
        password: "",
        username: "",
        trial: false,
        trial_info: {
          first_name: "",
          last_name: "",
          phone_number: "",
          job_title: "",
          company_name: "",
          country: "",
          developers: "",
        },
      },
      validationSchema,
      onSubmit,
    });
  const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
    form,
    error,
  );

  return (
    <SignInLayout>
      <header css={{ textAlign: "center", marginBottom: 32 }}>
        <CoderIcon
          css={(theme) => ({
            color: theme.palette.text.primary,
            fontSize: 64,
          })}
        />
        <h1
          css={{
            fontWeight: 400,
            margin: 0,
            marginTop: 16,
          }}
        >
          Welcome to <strong>Coder</strong>
        </h1>
        <div
          css={(theme) => ({
            marginTop: 12,
            color: theme.palette.text.secondary,
          })}
        >
          Let&lsquo;s create your first admin user account
        </div>
      </header>
      <VerticalForm onSubmit={form.handleSubmit}>
        <FormFields>
          <TextField
            autoFocus
            {...getFieldHelpers("username")}
            onChange={onChangeTrimmed(form)}
            autoComplete="username"
            fullWidth
            label={Language.usernameLabel}
          />
          <TextField
            {...getFieldHelpers("email")}
            onChange={onChangeTrimmed(form)}
            autoComplete="email"
            fullWidth
            label={Language.emailLabel}
          />
          <TextField
            {...getFieldHelpers("password")}
            autoComplete="current-password"
            fullWidth
            id="password"
            label={Language.passwordLabel}
            type="password"
          />

          <label
            htmlFor="trial"
            css={{
              display: "flex",
              cursor: "pointer",
              alignItems: "flex-start",
              gap: 4,
              marginTop: -4,
              marginBottom: 8,
            }}
          >
            <Checkbox
              id="trial"
              name="trial"
              checked={form.values.trial}
              onChange={form.handleChange}
              data-testid="trial"
              size="small"
            />

            <div css={{ fontSize: 14, paddingTop: 4 }}>
              <span css={{ display: "block", fontWeight: 600 }}>
                Start a 30-day free trial of Enterprise
              </span>
              <span
                css={(theme) => ({
                  display: "block",
                  fontSize: 13,
                  color: theme.palette.text.secondary,
                  lineHeight: "1.6",
                })}
              >
                Get access to high availability, template RBAC, audit logging,
                quotas, and more.
              </span>
              <Link
                href={docs("/enterprise")}
                target="_blank"
                css={{ marginTop: 4, display: "inline-block", fontSize: 13 }}
              >
                Read more
              </Link>
            </div>
          </label>

          {form.values.trial && (
            <>
              <Stack spacing={1.5} direction="row">
                <TextField
                  {...getFieldHelpers("trial_info.first_name")}
                  id="trial_info.first_name"
                  name="trial_info.first_name"
                  fullWidth
                  label={Language.firstNameLabel}
                />
                <TextField
                  {...getFieldHelpers("trial_info.last_name")}
                  id="trial_info.last_name"
                  name="trial_info.last_name"
                  fullWidth
                  label={Language.lastNameLabel}
                />
              </Stack>
              <TextField
                {...getFieldHelpers("trial_info.company_name")}
                id="trial_info.company_name"
                name="trial_info.company_name"
                fullWidth
                label={Language.companyLabel}
              />
              <TextField
                {...getFieldHelpers("trial_info.job_title")}
                id="trial_info.job_title"
                name="trial_info.job_title"
                fullWidth
                label={Language.jobTitleLabel}
              />
              <TextField
                {...getFieldHelpers("trial_info.phone_number")}
                id="trial_info.phone_number"
                name="trial_info.phone_number"
                fullWidth
                label={Language.phoneNumberLabel}
              />
              <Autocomplete
                autoHighlight
                options={countries}
                renderOption={(props, country) => (
                  <li {...props}>{`${country.flag} ${country.name}`}</li>
                )}
                getOptionLabel={(option) => option.name}
                onChange={(_, newValue) =>
                  form.setFieldValue("trial_info.country", newValue?.name)
                }
                css={{
                  "&:not(:has(label))": {
                    margin: 0,
                  },
                }}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    {...getFieldHelpers("trial_info.country")}
                    id="trial_info.country"
                    name="trial_info.country"
                    label={Language.countryLabel}
                    fullWidth
                    inputProps={{
                      ...params.inputProps,
                    }}
                    InputLabelProps={{ shrink: true }}
                  />
                )}
              />
              <TextField
                {...getFieldHelpers("trial_info.developers")}
                id="trial_info.developers"
                name="trial_info.developers"
                fullWidth
                label={Language.developersLabel}
                select
              >
                {numberOfDevelopersOptions.map((opt) => (
                  <MenuItem key={opt} value={opt}>
                    {opt}
                  </MenuItem>
                ))}
              </TextField>
              <div
                css={(theme) => ({
                  color: theme.palette.text.secondary,
                  fontSize: 11,
                  textAlign: "center",
                  marginTop: -5,
                  lineHeight: 1.5,
                })}
              >
                Complete the form to receive your trial license and be contacted
                about Coder products and solutions. The information you provide
                will be treated in accordance with the{" "}
                <Link
                  href="https://coder.com/legal/privacy-policy"
                  target="_blank"
                >
                  Coder Privacy Policy
                </Link>
                . Opt-out at any time.
              </div>
            </>
          )}

          <LoadingButton
            fullWidth
            loading={isLoading}
            type="submit"
            data-testid="create"
            size="large"
            variant="contained"
            color="primary"
          >
            {Language.create}
          </LoadingButton>
        </FormFields>
      </VerticalForm>
    </SignInLayout>
  );
};

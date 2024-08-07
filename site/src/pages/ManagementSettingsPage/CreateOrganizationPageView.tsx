import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import { isApiValidationError } from "api/errors";
import type { CreateOrganizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  Badges,
  DisabledBadge,
  PremiumBadge,
  EntitledBadge,
} from "components/Badges/Badges";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { PopoverPaywall } from "components/Paywall/PopoverPaywall";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { docs } from "utils/docs";
import {
  getFormHelpers,
  nameValidator,
  displayNameValidator,
  onChangeTrimmed,
} from "utils/formUtils";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE = `Please enter a description that is no longer than ${MAX_DESCRIPTION_CHAR_LIMIT} characters.`;

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  display_name: displayNameValidator("Display name"),
  description: Yup.string().max(
    MAX_DESCRIPTION_CHAR_LIMIT,
    MAX_DESCRIPTION_MESSAGE,
  ),
});

interface CreateOrganizationPageViewProps {
  error: unknown;
  onSubmit: (values: CreateOrganizationRequest) => Promise<void>;
  isEntitled: boolean;
}

export const CreateOrganizationPageView: FC<
  CreateOrganizationPageViewProps
> = ({ error, onSubmit, isEntitled }) => {
  const form = useFormik<CreateOrganizationRequest>({
    initialValues: {
      name: "",
      display_name: "",
      description: "",
      icon: "",
    },
    validationSchema,
    onSubmit,
  });
  const getFieldHelpers = getFormHelpers(form, error);

  return (
    <div>
      <SettingsHeader
        title="New Organization"
        description="Organize your deployment into multiple platform teams."
      />

      <Badges>
        {isEntitled ? <EntitledBadge /> : <DisabledBadge />}
        <Popover mode="hover">
          <PopoverTrigger>
            <span>
              <PremiumBadge />
            </span>
          </PopoverTrigger>
          <PopoverContent css={{ transform: "translateY(-28px)" }}>
            <PopoverPaywall
              message="Organizations"
              description="Organizations allow you to run a Coder deployment with multiple platform teams, all with unique use cases, templates, and even underlying infrastructure."
              // TODO: No documentation link yet.
              documentationLink={docs("/admin")}
              licenseType="premium"
            />
          </PopoverContent>
        </Popover>
      </Badges>

      {Boolean(error) && !isApiValidationError(error) && (
        <div css={{ marginBottom: 32 }}>
          <ErrorAlert error={error} />
        </div>
      )}

      <HorizontalForm
        onSubmit={form.handleSubmit}
        aria-label="Organization settings form"
      >
        <FormSection
          title="General info"
          description="Change the name or description of the organization."
        >
          <fieldset
            disabled={form.isSubmitting}
            css={{ border: "unset", padding: 0, margin: 0, width: "100%" }}
          >
            <FormFields>
              <TextField
                {...getFieldHelpers("name")}
                onChange={onChangeTrimmed(form)}
                autoFocus
                fullWidth
                label="Name"
              />
              <TextField
                {...getFieldHelpers("display_name")}
                fullWidth
                label="Display name"
              />
              <TextField
                {...getFieldHelpers("description")}
                multiline
                fullWidth
                label="Description"
                rows={2}
              />
              <IconField
                {...getFieldHelpers("icon")}
                onChange={onChangeTrimmed(form)}
                fullWidth
                onPickEmoji={(value) => form.setFieldValue("icon", value)}
              />
            </FormFields>
          </fieldset>
        </FormSection>
        <FormFooter isLoading={form.isSubmitting} />
      </HorizontalForm>
    </div>
  );
};

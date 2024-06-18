import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import { type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import * as Yup from "yup";
import {
  createOrganization,
  updateOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import type {
  Organization,
  UpdateOrganizationRequest,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { IconField } from "components/IconField/IconField";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import {
  getFormHelpers,
  nameValidator,
  displayNameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { useOrganizationSettings } from "./ManagementSettingsLayout";

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

interface OrganizationSettingsPageViewProps {
  org: Organization;
  error: unknown;
  onSubmit: (values: UpdateOrganizationRequest) => Promise<void>;

  onCreateOrg: (name: string) => void;
  onDeleteOrg: () => void;
}

export const OrganizationSettingsPageView: FC<
  OrganizationSettingsPageViewProps
> = ({ org, error, onSubmit, onCreateOrg, onDeleteOrg }) => {
  const form = useFormik<UpdateOrganizationRequest>({
    initialValues: {
      name: org.name,
      display_name: org.display_name,
      description: org.description,
      icon: org.icon,
    },
    validationSchema,
    onSubmit,
    enableReinitialize: true,
  });
  const getFieldHelpers = getFormHelpers(form, error);

  const [newOrgName, setNewOrgName] = useState("");

  return (
    <div>
      <PageHeader>
        <PageHeaderTitle>Organization settings</PageHeaderTitle>
      </PageHeader>

      <HorizontalForm
        onSubmit={form.handleSubmit}
        aria-label="Template settings form"
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

      {!org.is_default && (
        <Button
          css={styles.dangerButton}
          variant="contained"
          onClick={onDeleteOrg}
        >
          Delete this organization
        </Button>
      )}

      <Stack css={{ marginTop: 128 }}>
        <TextField
          label="New organization name"
          onChange={(event) => setNewOrgName(event.target.value)}
        />
        <Button onClick={() => onCreateOrg(newOrgName)}>
          Create new organization
        </Button>
      </Stack>
    </div>
  );
};

const styles = {
  dangerButton: (theme) => ({
    "&.MuiButton-contained": {
      backgroundColor: theme.roles.danger.fill.solid,
      borderColor: theme.roles.danger.fill.outline,

      "&:not(.MuiLoadingButton-loading)": {
        color: theme.roles.danger.fill.text,
      },

      "&:hover:not(:disabled)": {
        backgroundColor: theme.roles.danger.hover.fill.solid,
        borderColor: theme.roles.danger.hover.fill.outline,
      },

      "&.Mui-disabled": {
        backgroundColor: theme.roles.danger.disabled.background,
        borderColor: theme.roles.danger.disabled.outline,

        "&:not(.MuiLoadingButton-loading)": {
          color: theme.roles.danger.disabled.fill.text,
        },
      },
    },
  }),
} satisfies Record<string, Interpolation<Theme>>;

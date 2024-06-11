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
import type { UpdateOrganizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import { Margins } from "components/Margins/Margins";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import {
  getFormHelpers,
  nameValidator,
  displayNameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { useOrganizationSettings } from "./OrganizationSettingsLayout";

const MAX_DESCRIPTION_CHAR_LIMIT = 128;
const MAX_DESCRIPTION_MESSAGE =
  "Please enter a description that is no longer than 128 characters.";

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    name: nameValidator("Name"),
    display_name: displayNameValidator("Display name"),
    description: Yup.string().max(
      MAX_DESCRIPTION_CHAR_LIMIT,
      MAX_DESCRIPTION_MESSAGE,
    ),
  });

const OrganizationSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const addOrganizationMutation = useMutation(createOrganization(queryClient));
  const updateOrganizationMutation = useMutation(
    updateOrganization(queryClient),
  );
  const deleteOrganizationMutation = useMutation(
    deleteOrganization(queryClient),
  );

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const org = organizations.find((org) => org.id === currentOrganizationId)!;

  const error =
    addOrganizationMutation.error ?? deleteOrganizationMutation.error;

  const form = useFormik<UpdateOrganizationRequest>({
    initialValues: {
      name: org.name,
      display_name: org.display_name,
      description: org.description,
    },
    validationSchema: getValidationSchema(),
    onSubmit: (values) =>
      updateOrganizationMutation.mutateAsync({ orgId: org.id, req: values }),
    enableReinitialize: true,
  });
  const getFieldHelpers = getFormHelpers(form, error);

  const [newOrgName, setNewOrgName] = useState("");

  return (
    <Margins verticalMargin={18}>
      {Boolean(error) && <ErrorAlert error={error} />}

      <PageHeader css={{ paddingTop: 0 }}>
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
          <FormFields>
            <TextField
              {...getFieldHelpers("name")}
              disabled={form.isSubmitting}
              onChange={onChangeTrimmed(form)}
              autoFocus
              fullWidth
              label="Name"
            />
            <TextField
              {...getFieldHelpers("display_name")}
              disabled={form.isSubmitting}
              fullWidth
              label="Display name"
            />
            <TextField
              {...getFieldHelpers("description")}
              multiline
              disabled={form.isSubmitting}
              fullWidth
              label="Description"
              rows={2}
            />
          </FormFields>
        </FormSection>
        <FormFooter isLoading={form.isSubmitting} />
      </HorizontalForm>

      {!org.is_default && (
        <Button
          css={styles.dangerButton}
          variant="contained"
          onClick={() =>
            deleteOrganizationMutation.mutate(currentOrganizationId)
          }
        >
          Delete this organization
        </Button>
      )}

      <hr />
      <TextField
        label="New organization name"
        onChange={(event) => setNewOrgName(event.target.value)}
      />
      <Button
        onClick={() => addOrganizationMutation.mutate({ name: newOrgName })}
      >
        Create new team
      </Button>
    </Margins>
  );
};

export default OrganizationSettingsPage;

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

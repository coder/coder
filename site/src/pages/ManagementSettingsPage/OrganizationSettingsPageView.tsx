import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import { useFormik } from "formik";
import { type FC, useState } from "react";
import * as Yup from "yup";
import type {
  Organization,
  UpdateOrganizationRequest,
} from "api/typesGenerated";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import {
  getFormHelpers,
  nameValidator,
  displayNameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import { HorizontalContainer, HorizontalSection } from "./Horizontal";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";

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

  onDeleteOrg: () => void;
}

export const OrganizationSettingsPageView: FC<
  OrganizationSettingsPageViewProps
> = ({ org, error, onSubmit, onDeleteOrg }) => {
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

  const [isDeleting, setIsDeleting] = useState(false);

  return (
    <div>
      <PageHeader>
        <PageHeaderTitle>Organization settings</PageHeaderTitle>
      </PageHeader>

      <HorizontalForm
        onSubmit={form.handleSubmit}
        aria-label="Organization settings form"
      >
        <FormSection
          title="Info"
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
        <HorizontalContainer css={{ marginTop: 48 }}>
          <HorizontalSection
            title="Settings"
            description="Change or delete your organization."
          >
            <div
              css={(theme) => ({
                display: "flex",
                backgroundColor: theme.roles.danger.background,
                alignItems: "center",
                justifyContent: "space-between",
                border: `1px solid ${theme.roles.danger.outline}`,
                borderRadius: 8,
                padding: 12,
                paddingLeft: 18,
                gap: 8,
                lineHeight: "18px",
                flexGrow: 1,

                "& .option": {
                  color: theme.roles.danger.fill.solid,
                  "&.Mui-checked": {
                    color: theme.roles.danger.fill.solid,
                  },
                },

                "& .info": {
                  fontSize: 14,
                  fontWeight: 600,
                  color: theme.roles.danger.text,
                },
              })}
            >
              <span>Deleting an organization is irreversible.</span>
              <Button
                css={styles.dangerButton}
                variant="contained"
                onClick={() => setIsDeleting(true)}
              >
                Delete this organization
              </Button>
            </div>
          </HorizontalSection>
        </HorizontalContainer>
      )}

      <DeleteDialog
        isOpen={isDeleting}
        onConfirm={onDeleteOrg}
        onCancel={() => setIsDeleting(false)}
        entity="organization"
        name={org.name}
      />
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

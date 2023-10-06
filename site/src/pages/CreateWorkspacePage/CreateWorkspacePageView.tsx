import TextField from "@mui/material/TextField";
import * as TypesGen from "api/typesGenerated";
import { UserAutocomplete } from "components/UserAutocomplete/UserAutocomplete";
import { FormikContextType, useFormik } from "formik";
import { FC, useEffect, useState } from "react";
import {
  getFormHelpers,
  nameValidator,
  onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { SelectedTemplate } from "./SelectedTemplate";
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/Form/Form";
import { makeStyles } from "@mui/styles";
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";
import {
  ImmutableTemplateParametersSection,
  MutableTemplateParametersSection,
} from "components/TemplateParameters/TemplateParameters";
import { CreateWSPermissions } from "xServices/createWorkspace/createWorkspaceXService";
import { ExternalAuth } from "./ExternalAuth";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Stack } from "components/Stack/Stack";
import { type ExternalAuthPollingState } from "./CreateWorkspacePage";
import { useSearchParams } from "react-router-dom";

export interface CreateWorkspacePageViewProps {
  error: unknown;
  defaultName: string;
  defaultOwner: TypesGen.User;
  template: TypesGen.Template;
  versionId?: string;
  externalAuth: TypesGen.TemplateVersionExternalAuth[];
  externalAuthPollingState: ExternalAuthPollingState;
  startPollingExternalAuth: () => void;
  parameters: TypesGen.TemplateVersionParameter[];
  defaultBuildParameters: TypesGen.WorkspaceBuildParameter[];
  permissions: CreateWSPermissions;
  creatingWorkspace: boolean;
  onCancel: () => void;
  onSubmit: (
    req: TypesGen.CreateWorkspaceRequest,
    owner: TypesGen.User,
  ) => void;
}

export const CreateWorkspacePageView: FC<CreateWorkspacePageViewProps> = ({
  error,
  defaultName,
  defaultOwner,
  template,
  versionId,
  externalAuth,
  externalAuthPollingState,
  startPollingExternalAuth,
  parameters,
  defaultBuildParameters,
  permissions,
  creatingWorkspace,
  onSubmit,
  onCancel,
}) => {
  const styles = useStyles();
  const [owner, setOwner] = useState(defaultOwner);
  const { verifyExternalAuth, externalAuthErrors } =
    useExternalAuthVerification(externalAuth);
  const [searchParams] = useSearchParams();
  const disabledParamsList = searchParams?.get("disable_params")?.split(",");

  const form: FormikContextType<TypesGen.CreateWorkspaceRequest> =
    useFormik<TypesGen.CreateWorkspaceRequest>({
      initialValues: {
        name: defaultName,
        template_id: template.id,
        rich_parameter_values: getInitialRichParameterValues(
          parameters,
          defaultBuildParameters,
        ),
      },
      validationSchema: Yup.object({
        name: nameValidator("Workspace Name"),
        rich_parameter_values: useValidationSchemaForRichParameters(parameters),
      }),
      enableReinitialize: true,
      onSubmit: (request) => {
        if (!verifyExternalAuth()) {
          form.setSubmitting(false);
          return;
        }

        onSubmit(request, owner);
      },
    });

  useEffect(() => {
    if (error) {
      window.scrollTo(0, 0);
    }
  }, [error]);

  const getFieldHelpers = getFormHelpers<TypesGen.CreateWorkspaceRequest>(
    form,
    error,
  );

  return (
    <FullPageHorizontalForm title="New workspace" onCancel={onCancel}>
      <HorizontalForm onSubmit={form.handleSubmit}>
        {Boolean(error) && <ErrorAlert error={error} />}
        {/* General info */}
        <FormSection
          title="General"
          description="The template and name of your new workspace."
        >
          <FormFields>
            <SelectedTemplate template={template} />
            {versionId && versionId !== template.active_version_id && (
              <Stack spacing={1} className={styles.hasDescription}>
                <TextField
                  disabled
                  fullWidth
                  value={versionId}
                  label="Version ID"
                />
                <span className={styles.description}>
                  This parameter has been preset, and cannot be modified.
                </span>
              </Stack>
            )}
            <TextField
              {...getFieldHelpers("name")}
              disabled={form.isSubmitting}
              onChange={onChangeTrimmed(form)}
              autoFocus
              fullWidth
              label="Workspace Name"
            />
          </FormFields>
        </FormSection>

        {permissions.createWorkspaceForUser && (
          <FormSection
            title="Workspace Owner"
            description="Only admins can create workspace for other users."
          >
            <FormFields>
              <UserAutocomplete
                value={owner}
                onChange={(user) => {
                  setOwner(user ?? defaultOwner);
                }}
                label="Owner"
                size="medium"
              />
            </FormFields>
          </FormSection>
        )}

        {externalAuth && externalAuth.length > 0 && (
          <FormSection
            title="External Authentication"
            description="This template requires authentication to external services."
          >
            <FormFields>
              {externalAuth.map((auth) => (
                <ExternalAuth
                  key={auth.id}
                  authenticateURL={auth.authenticate_url}
                  authenticated={auth.authenticated}
                  externalAuthPollingState={externalAuthPollingState}
                  startPollingExternalAuth={startPollingExternalAuth}
                  displayName={auth.display_name}
                  displayIcon={auth.display_icon}
                  error={externalAuthErrors[auth.id]}
                />
              ))}
            </FormFields>
          </FormSection>
        )}

        {parameters && (
          <>
            <MutableTemplateParametersSection
              templateParameters={parameters}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
                  },
                  disabled:
                    disabledParamsList?.includes(
                      parameter.name.toLowerCase().replace(/ /g, "_"),
                    ) || form.isSubmitting,
                };
              }}
            />
            <ImmutableTemplateParametersSection
              templateParameters={parameters}
              classes={{ root: styles.warningSection }}
              getInputProps={(parameter, index) => {
                return {
                  ...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  ),
                  onChange: async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
                  },
                  disabled:
                    disabledParamsList?.includes(
                      parameter.name.toLowerCase().replace(/ /g, "_"),
                    ) || form.isSubmitting,
                };
              }}
            />
          </>
        )}

        <FormFooter
          onCancel={onCancel}
          isLoading={creatingWorkspace}
          submitLabel="Create Workspace"
        />
      </HorizontalForm>
    </FullPageHorizontalForm>
  );
};

type ExternalAuthErrors = Record<string, string>;

const useExternalAuthVerification = (
  externalAuth: TypesGen.TemplateVersionExternalAuth[],
) => {
  const [externalAuthErrors, setExternalAuthErrors] =
    useState<ExternalAuthErrors>({});

  // Clear errors when externalAuth is refreshed
  useEffect(() => {
    setExternalAuthErrors({});
  }, [externalAuth]);

  const verifyExternalAuth = () => {
    const errors: ExternalAuthErrors = {};

    for (let i = 0; i < externalAuth.length; i++) {
      const auth = externalAuth.at(i);
      if (!auth) {
        continue;
      }
      if (!auth.authenticated) {
        errors[auth.id] = "You must authenticate to create a workspace!";
      }
    }

    setExternalAuthErrors(errors);
    const isValid = Object.keys(errors).length === 0;
    return isValid;
  };

  return {
    externalAuthErrors,
    verifyExternalAuth,
  };
};

const useStyles = makeStyles((theme) => ({
  hasDescription: {
    paddingBottom: theme.spacing(2),
  },
  description: {
    fontSize: 13,
    color: theme.palette.text.secondary,
  },
  warningText: {
    color: theme.palette.warning.light,
  },
  warningSection: {
    border: `1px solid ${theme.palette.warning.light}`,
    borderRadius: 8,
    backgroundColor: theme.palette.background.paper,
    padding: theme.spacing(10),
    marginLeft: theme.spacing(-10),
    marginRight: theme.spacing(-10),
  },
}));

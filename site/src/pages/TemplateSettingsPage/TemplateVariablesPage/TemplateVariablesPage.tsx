import { useCallback, type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import {
  createAndBuildTemplateVersion,
  templateVersion,
  templateVersionVariables,
  updateActiveTemplateVersion,
} from "api/queries/templates";
import type {
  CreateTemplateVersionRequest,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateVariablesPageView } from "./TemplateVariablesPageView";

export const TemplateVariablesPage: FC = () => {
  const { template: templateName } = useParams() as {
    organization: string;
    template: string;
  };
  const { organizationId } = useDashboard();
  const { template } = useTemplateSettings();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const versionId = template.active_version_id;

  const {
    data: version,
    error: versionError,
    isLoading: isVersionLoading,
  } = useQuery({ ...templateVersion(versionId), keepPreviousData: true });
  const {
    data: variables,
    error: variablesError,
    isLoading: isVariablesLoading,
  } = useQuery({
    ...templateVersionVariables(versionId),
    keepPreviousData: true,
  });

  const {
    mutateAsync: sendCreateAndBuildTemplateVersion,
    error: buildError,
    isLoading: isBuilding,
  } = useMutation(createAndBuildTemplateVersion(organizationId));
  const {
    mutateAsync: sendUpdateActiveTemplateVersion,
    error: publishError,
    isLoading: isPublishing,
  } = useMutation(updateActiveTemplateVersion(template, queryClient));

  const publishVersion = useCallback(
    async (versionId: string) => {
      await sendUpdateActiveTemplateVersion(versionId);
      displaySuccess("Template updated successfully");
    },
    [sendUpdateActiveTemplateVersion],
  );

  const buildVersion = useCallback(
    async (req: CreateTemplateVersionRequest) => {
      const newVersion = await sendCreateAndBuildTemplateVersion(req);
      await publishVersion(newVersion.id);
    },
    [sendCreateAndBuildTemplateVersion, publishVersion],
  );

  const isSubmitting = Boolean(isBuilding || isPublishing);

  const error = versionError ?? variablesError;
  if (error) {
    return <ErrorAlert error={error} />;
  }

  if (isVersionLoading || isVariablesLoading) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "Template variables"])}</title>
      </Helmet>

      <TemplateVariablesPageView
        isSubmitting={isSubmitting}
        templateVersion={version}
        templateVariables={variables}
        errors={{
          buildError,
          publishError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`);
        }}
        onSubmit={async (formData) => {
          const request = filterEmptySensitiveVariables(formData, variables);
          await buildVersion(request);
        }}
      />
    </>
  );
};

const filterEmptySensitiveVariables = (
  request: CreateTemplateVersionRequest,
  templateVariables?: TemplateVersionVariable[],
): CreateTemplateVersionRequest => {
  const filtered: VariableValue[] = [];

  if (!templateVariables) {
    return request;
  }

  if (request.user_variable_values) {
    request.user_variable_values.forEach((variableValue) => {
      const templateVariable = templateVariables.find(
        (t) => t.name === variableValue.name,
      );
      if (
        templateVariable &&
        templateVariable.sensitive &&
        variableValue.value === ""
      ) {
        return;
      }
      filtered.push(variableValue);
    });
  }

  return {
    ...request,
    user_variable_values: filtered,
  };
};

export default TemplateVariablesPage;

import { useMachine } from "@xstate/react";
import {
  CreateTemplateVersionRequest,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useOrganizationId } from "hooks/useOrganizationId";
import { useEffect, type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateVariablesPageView } from "./TemplateVariablesPageView";
import {
  createTemplateVersion,
  templateVersion,
  templateVersionVariables,
  updateActiveTemplateVersion,
} from "api/queries/templates";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";

export const TemplateVariablesPage: FC = () => {
  const { template: templateName } = useParams() as {
    organization: string;
    template: string;
  };
  const orgId = useOrganizationId();
  const { template } = useTemplateSettings();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const versionId = template.active_version_id;

  const [newVersionId, setNewVersionId] = useState<string | null>(null);

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

  const createVersionMutation = createTemplateVersion(orgId, queryClient);
  const { mutate: sendCreateTemplateVersion, error: createVersionError } =
    useMutation({
      ...createVersionMutation,
      onSuccess: (data) => {
        setNewVersionId(data.id);
      },
    });
  const publishMutation = updateActiveTemplateVersion(template, queryClient);
  const { mutate: publishVersion, error: publishError } = useMutation({
    ...publishMutation,
    onSuccess: () => {
      publishMutation.onSuccess();
      displaySuccess("Template updated successfully");
    },
    onSettled: () => {
      setNewVersionId(null);
    },
  });

  const {
    data: status,
    error: statusError,
    refetch: refetchStatus,
    isRefetching,
  } = useQuery(
    newVersionId ? templateVersion(newVersionId) : { enabled: false },
  );

  const isSubmitting = Boolean(newVersionId && !statusError);

  // Poll build status while we're updating the template
  useEffect(() => {
    if (!newVersionId || !status || isRefetching) {
      return;
    }

    const jobStatus = status.job.status;
    if (jobStatus === "pending" || jobStatus === "running") {
      setTimeout(() => refetchStatus(), 2_000);
      return;
    }

    publishVersion(newVersionId);
  }, [newVersionId, status, isRefetching]);

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
          createVersionError,
          statusError,
          jobError: status?.job.error,
          publishError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`);
        }}
        onSubmit={(formData) => {
          const request = filterEmptySensitiveVariables(formData, variables);
          sendCreateTemplateVersion(request);
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

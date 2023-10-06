import { useDashboard } from "components/Dashboard/DashboardProvider";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { Loader } from "components/Loader/Loader";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { CreateTemplateData } from "xServices/createTemplate/createTemplateXService";
import { CreateTemplateForm } from "./CreateTemplateForm";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  JobError,
  createTemplate,
  templateByName,
  templateVersion,
  templateVersionVariables,
} from "api/queries/templates";
import { ProvisionerType } from "api/typesGenerated";
import { calculateAutostopRequirementDaysValue } from "utils/schedule";
import { templateVersionLogs } from "api/queries/templateVersions";

const provisioner: ProvisionerType =
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Playwright needs to use a different provisioner type!
  typeof (window as any).playwright !== "undefined" ? "echo" : "terraform";

const CreateTemplatePage: FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  // const organizationId = useOrganizationId();
  // const [state, send] = useMachine(createTemplateMachine, {
  //   context: {
  //     organizationId,
  //     exampleId: searchParams.get("exampleId"),
  //     templateNameToCopy: searchParams.get("fromTemplate"),
  //   },
  //   actions: {
  //     onCreate: (_, { data }) => {
  //       navigate(`/templates/${data.name}`);
  //     },
  //   },
  // });

  // const { starterTemplate, error, file, jobError, jobLogs, variables } =
  //   state.context;
  // const shouldDisplayForm = !state.hasTag("loading");
  // const { entitlements } = useDashboard();
  // const allowAdvancedScheduling =
  //   entitlements.features["advanced_template_scheduling"].enabled;
  // // Requires the template RBAC feature, otherwise disabling everyone access
  // // means no one can access.
  // const allowDisableEveryoneAccess =
  //   entitlements.features["template_rbac"].enabled;
  // const allowAutostopRequirement =
  //   entitlements.features["template_autostop_requirement"].enabled;

  const onCancel = () => {
    navigate(-1);
  };

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Template")}</title>
      </Helmet>

      <FullPageHorizontalForm title="Create Template" onCancel={onCancel}>
        {searchParams.has("fromTemplate") ? (
          <DuplicateTemplateView />
        ) : searchParams.has("exampleId") ? (
          <ImportStaterTemplateView />
        ) : (
          <UploadTemplateView />
        )}
        {/* {state.hasTag("loading") && <Loader />}

        <Stack spacing={6}>
          {Boolean(error) && !isApiValidationError(error) && (
            <ErrorAlert error={error} />
          )}

          {shouldDisplayForm && (
            <CreateTemplateForm
              copiedTemplate={state.context.copiedTemplate}
              allowAdvancedScheduling={allowAdvancedScheduling}
              allowDisableEveryoneAccess={allowDisableEveryoneAccess}
              allowAutostopRequirement={allowAutostopRequirement}
              error={error}
              starterTemplate={starterTemplate}
              isSubmitting={state.hasTag("submitting")}
              variables={variables}
              onCancel={onCancel}
              onSubmit={(data) => {
                send({
                  type: "CREATE",
                  data,
                });
              }}
              upload={{
                file,
                isUploading: state.matches("uploading"),
                onRemove: () => {
                  send("REMOVE_FILE");
                },
                onUpload: (file) => {
                  send({ type: "UPLOAD_FILE", file });
                },
              }}
              jobError={jobError}
              logs={jobLogs}
            />
          )}
        </Stack> */}
      </FullPageHorizontalForm>
    </>
  );
};

const DuplicateTemplateView = () => {
  const navigate = useNavigate();
  const organizationId = useOrganizationId();
  const [searchParams] = useSearchParams();
  const templateByNameQuery = useQuery(
    templateByName(organizationId, searchParams.get("fromTemplate")!),
  );
  const activeVersionId =
    templateByNameQuery.data?.template.active_version_id ?? "";
  const templateVersionQuery = useQuery({
    ...templateVersion(activeVersionId),
    enabled: templateByNameQuery.isSuccess,
  });
  const templateVersionVariablesQuery = useQuery({
    ...templateVersionVariables(activeVersionId),
    enabled: templateByNameQuery.isSuccess,
  });
  const isLoading =
    templateByNameQuery.isLoading ||
    templateVersionQuery.isLoading ||
    templateVersionVariablesQuery.isLoading;
  const loadingError =
    templateByNameQuery.error ||
    templateVersionQuery.error ||
    templateVersionVariablesQuery.error;

  const formEntitlements = useFormEntitlements();

  const createTemplateMutation = useMutation(createTemplate());
  const createError = createTemplateMutation.error;
  const isJobError = createError instanceof JobError;
  const templateVersionLogsQuery = useQuery({
    ...templateVersionLogs(isJobError ? createError.version.id : ""),
    enabled: isJobError,
  });

  if (isLoading) {
    return <Loader />;
  }

  if (loadingError) {
    return <ErrorAlert error={loadingError} />;
  }

  return (
    <CreateTemplateForm
      {...formEntitlements}
      copiedTemplate={templateByNameQuery.data!.template}
      error={createTemplateMutation.error}
      isSubmitting={createTemplateMutation.isLoading}
      variables={templateVersionVariablesQuery.data}
      onCancel={() => navigate(-1)}
      onSubmit={async (formData) => {
        const template = await createTemplateMutation.mutateAsync({
          organizationId,
          version: {
            storage_method: "file",
            file_id: templateVersionQuery.data!.job.file_id,
            provisioner: provisioner,
            tags: {},
          },
          data: prepareData(formData),
        });
        navigate(`/templates/${template.name}`);
      }}
      jobError={isJobError ? createError.job.error : undefined}
      logs={templateVersionLogsQuery.data}
    />
  );
};

const ImportStaterTemplateView = () => {
  return <div>Import</div>;
};

const UploadTemplateView = () => {
  return <div>Upload</div>;
};

const useFormEntitlements = () => {
  const { entitlements } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // Requires the template RBAC feature, otherwise disabling everyone access
  // means no one can access.
  const allowDisableEveryoneAccess =
    entitlements.features["template_rbac"].enabled;
  const allowAutostopRequirement =
    entitlements.features["template_autostop_requirement"].enabled;

  return {
    allowAdvancedScheduling,
    allowDisableEveryoneAccess,
    allowAutostopRequirement,
  };
};

const prepareData = (formData: CreateTemplateData) => {
  const {
    default_ttl_hours,
    max_ttl_hours,
    parameter_values_by_name,
    allow_everyone_group_access,
    autostop_requirement_days_of_week,
    autostop_requirement_weeks,
    ...safeTemplateData
  } = formData;

  return {
    ...safeTemplateData,
    disable_everyone_group_access: !formData.allow_everyone_group_access,
    default_ttl_ms: formData.default_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
    max_ttl_ms: formData.max_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
    autostop_requirement: {
      days_of_week: calculateAutostopRequirementDaysValue(
        formData.autostop_requirement_days_of_week,
      ),
      weeks: formData.autostop_requirement_weeks,
    },
  };
};

export default CreateTemplatePage;

import type {
  Entitlements,
  ProvisionerType,
  TemplateExample,
  VariableValue,
} from "api/typesGenerated";
import { calculateAutostopRequirementDaysValue } from "utils/schedule";
import type { CreateTemplateData } from "./CreateTemplateForm";

const provisioner: ProvisionerType =
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Playwright needs to use a different provisioner type!
  typeof (window as any).playwright !== "undefined" ? "echo" : "terraform";

export const newTemplate = (formData: CreateTemplateData) => {
  const { autostop_requirement_days_of_week, autostop_requirement_weeks } =
    formData;

  const safeTemplateData = {
    name: formData.name,
    display_name: formData.display_name,
    description: formData.description,
    icon: formData.icon,
    allow_user_autostart: formData.allow_user_autostart,
    allow_user_autostop: formData.allow_user_autostop,
    allow_user_cancel_workspace_jobs: formData.allow_user_cancel_workspace_jobs,
    user_variable_values: formData.user_variable_values,
    allow_everyone_group_access: formData.allow_everyone_group_access,
  };

  return {
    ...safeTemplateData,
    disable_everyone_group_access: !formData.allow_everyone_group_access,
    default_ttl_ms: formData.default_ttl_hours * 60 * 60 * 1000, // Convert hours to ms
    autostop_requirement: {
      days_of_week: calculateAutostopRequirementDaysValue(
        autostop_requirement_days_of_week,
      ),
      weeks: autostop_requirement_weeks,
    },
    autostart_requirement: {
      days_of_week: formData.autostart_requirement_days_of_week,
    },
    require_active_version: false,
  };
};

export const getFormPermissions = (entitlements: Entitlements) => {
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;

  return {
    allowAdvancedScheduling,
  };
};

export const firstVersionFromFile = (
  fileId: string,
  variables: VariableValue[] | undefined,
) => {
  return {
    storage_method: "file" as const,
    provisioner: provisioner,
    user_variable_values: variables,
    file_id: fileId,
    tags: {},
  };
};

export const firstVersionFromExample = (
  example: TemplateExample,
  variables: VariableValue[] | undefined,
) => {
  return {
    storage_method: "file" as const,
    provisioner: provisioner,
    user_variable_values: variables,
    example_id: example.id,
    tags: {},
  };
};

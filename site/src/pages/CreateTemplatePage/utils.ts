import {
  Entitlements,
  ProvisionerType,
  TemplateExample,
  VariableValue,
} from "api/typesGenerated";
import { calculateAutostopRequirementDaysValue } from "utils/schedule";
import { CreateTemplateData } from "./CreateTemplateForm";

const provisioner: ProvisionerType =
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Playwright needs to use a different provisioner type!
  typeof (window as any).playwright !== "undefined" ? "echo" : "terraform";

export const newTemplate = (formData: CreateTemplateData) => {
  const {
    default_ttl_hours,
    max_ttl_hours,
    parameter_values_by_name,
    allow_everyone_group_access,
    autostart_requirement_days_of_week,
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
    autostart_requirement: {
      days_of_week: autostart_requirement_days_of_week,
    },
    require_active_version: false,
  };
};

export const getFormPermissions = (entitlements: Entitlements) => {
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

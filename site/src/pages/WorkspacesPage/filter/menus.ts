import { getTemplates } from "api/api";
import type { WorkspaceStatus } from "api/typesGenerated";
import {
  useFilterMenu,
  type UseFilterMenuOptions,
} from "components/Filter/menu";
import { getDisplayWorkspaceStatus } from "utils/workspace";
import type { StatusOption, TemplateOption } from "./options";

export const useTemplateFilterMenu = ({
  value,
  onChange,
  organizationId,
}: { organizationId: string } & Pick<
  UseFilterMenuOptions<TemplateOption>,
  "value" | "onChange"
>) => {
  return useFilterMenu({
    onChange,
    value,
    id: "template",
    getSelectedOption: async () => {
      // Show all templates including deprecated
      const templates = await getTemplates(organizationId);
      const template = templates.find((template) => template.name === value);
      if (template) {
        return {
          label:
            template.display_name !== ""
              ? template.display_name
              : template.name,
          value: template.name,
          icon: template.icon,
        };
      }
      return null;
    },
    getOptions: async (query) => {
      // Show all templates including deprecated
      const templates = await getTemplates(organizationId);
      const filteredTemplates = templates.filter(
        (template) =>
          template.name.toLowerCase().includes(query.toLowerCase()) ||
          template.display_name.toLowerCase().includes(query.toLowerCase()),
      );
      return filteredTemplates.map((template) => ({
        label:
          template.display_name !== "" ? template.display_name : template.name,
        value: template.name,
        icon: template.icon,
      }));
    },
  });
};

export type TemplateFilterMenu = ReturnType<typeof useTemplateFilterMenu>;

export const useStatusFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<StatusOption>, "value" | "onChange">) => {
  const statusesToFilter: WorkspaceStatus[] = [
    "running",
    "stopped",
    "failed",
    "pending",
  ];
  const statusOptions = statusesToFilter.map((status) => {
    const display = getDisplayWorkspaceStatus(status);
    return {
      label: display.text,
      value: status,
      color: display.type ?? "warning",
    } as StatusOption;
  });
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      statusOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => statusOptions,
  });
};

export type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>;

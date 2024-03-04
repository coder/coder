import type { BaseOption } from "components/Filter/options";
import type { ThemeRole } from "theme/roles";

export type StatusOption = BaseOption & {
  color: ThemeRole;
};

export type TemplateOption = BaseOption & {
  icon?: string;
};

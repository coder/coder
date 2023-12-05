import { BaseOption } from "components/Filter/options";
import type { ThemeRole } from "theme/experimental";

export type StatusOption = BaseOption & {
  color: ThemeRole;
};

export type TemplateOption = BaseOption & {
  icon?: string;
};

import { BaseOption } from "components/Filter/options";

export type StatusOption = BaseOption & {
  color: string;
};

export type TemplateOption = BaseOption & {
  icon?: string;
};

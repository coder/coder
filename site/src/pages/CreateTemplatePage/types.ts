import { CreateTemplateOptions } from "api/queries/templates";

export type CreateTemplatePageViewProps = {
  onCreateTemplate: (options: CreateTemplateOptions) => Promise<void>;
  error: unknown;
  isCreating: boolean;
};

import type { CreateTemplateOptions } from "api/queries/templates";

export type CreateTemplatePageViewProps = {
  onCreateTemplate: (options: CreateTemplateOptions) => Promise<void>;
  onOpenBuildLogsDrawer: () => void;
  variablesSectionRef: React.RefObject<HTMLDivElement>;
  error: unknown;
  isCreating: boolean;
};

export const createWorkspaceModes = ["form", "auto", "duplicate"] as const;

export type CreateWorkspaceMode = (typeof createWorkspaceModes)[number];

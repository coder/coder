export const chatProjectsFamilyKey = ["chat-projects"] as const;

export const chatProjectsKey = (organizationId: string) =>
	[...chatProjectsFamilyKey, organizationId] as const;

export const chatProjectKey = (projectId: string) =>
	[...chatProjectsFamilyKey, "project", projectId] as const;

export const chatProjectMemoriesKey = (projectId: string) =>
	[...chatProjectsFamilyKey, projectId, "memories"] as const;

import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const workspaceSkillsKey = (workspaceId: string) =>
	["workspace", workspaceId, "skills"] as const;

export const workspaceSkills = (workspaceId: string) => ({
	queryKey: workspaceSkillsKey(workspaceId),
	queryFn: (): Promise<TypesGen.WorkspaceSkillMetadata[]> =>
		API.experimental.getWorkspaceSkills(workspaceId),
});

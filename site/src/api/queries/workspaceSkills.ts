import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const workspaceSkillsKey = (workspaceId: string) =>
	["workspace-skills", workspaceId] as const;

export const workspaceSkills = (workspaceId: string) => ({
	queryKey: workspaceSkillsKey(workspaceId),
	queryFn: (): Promise<TypesGen.WorkspaceSkillMetadata[]> =>
		API.experimental.getWorkspaceSkills(workspaceId),
});

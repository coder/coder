import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

export const agentMemoriesKey = (user = "me") =>
	["agent-memories", user] as const;

export const agentMemoryDefault = (user = "me") => ({
	queryKey: [...agentMemoriesKey(user), "default"] as const,
	queryFn: () => API.experimental.getDefaultAgentMemory(user),
});

export const agentMemoryChildren = (
	directory: string,
	offset: number,
	user = "me",
) => ({
	queryKey: [...agentMemoriesKey(user), "children", directory, offset] as const,
	queryFn: () =>
		API.experimental.getAgentMemoryChildren(user, directory, offset),
});

export const agentMemory = (memoryID: string, user = "me") => ({
	queryKey: [...agentMemoriesKey(user), "memory", memoryID] as const,
	queryFn: () => API.experimental.getAgentMemory(user, memoryID),
});

type UpdateAgentMemoryArgs = {
	memoryID: string;
	request: TypesGen.UpdateAgentMemoryRequest;
};

export const updateAgentMemory = (queryClient: QueryClient, user = "me") => ({
	mutationFn: ({ memoryID, request }: UpdateAgentMemoryArgs) =>
		API.experimental.updateAgentMemory(user, memoryID, request),
	onSuccess: (memory: TypesGen.AgentMemory) => {
		queryClient.setQueryData(
			[...agentMemoriesKey(user), "memory", memory.id],
			memory,
		);
		queryClient.setQueryData<TypesGen.AgentMemory | undefined>(
			agentMemoryDefault(user).queryKey,
			(current) => (current?.id === memory.id ? memory : current),
		);
	},
});

export const deleteAgentMemory = (queryClient: QueryClient, user = "me") => ({
	mutationFn: (memory: TypesGen.AgentMemory) =>
		API.experimental.deleteAgentMemory(user, memory.id),
	onSuccess: (_data: unknown, memory: TypesGen.AgentMemory) => {
		queryClient.removeQueries({
			queryKey: [...agentMemoriesKey(user), "memory", memory.id],
			exact: true,
		});
		queryClient.removeQueries({
			queryKey: agentMemoryDefault(user).queryKey,
			exact: true,
		});
	},
});

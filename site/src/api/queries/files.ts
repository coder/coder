import { API } from "#/api/api";
import { readTemplateArchive } from "#/utils/templateArchive";

export const uploadFile = () => {
	return {
		mutationFn: API.uploadFile,
	};
};

/** Parses a full immutable source archive, including local modules. */
export const templateArchive = (fileId: string) => ({
	queryKey: [...file(fileId).queryKey, "archive"],
	queryFn: async () => readTemplateArchive(await API.getFile(fileId)),
	staleTime: Number.POSITIVE_INFINITY,
});

export const file = (fileId: string) => {
	return {
		queryKey: ["files", fileId],
		queryFn: () => API.getFile(fileId),
	};
};

import { client } from "api/api";

export const uploadFile = () => {
  return {
    mutationFn: client.api.uploadFile,
  };
};

export const file = (fileId: string) => {
  return {
    queryKey: ["files", fileId],
    queryFn: () => client.api.getFile(fileId),
  };
};

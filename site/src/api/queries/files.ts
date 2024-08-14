import { API } from "api/api";

export const uploadFile = () => {
  return {
    mutationFn: API.uploadFile,
  };
};

export const file = (fileId: string) => {
  return {
    queryKey: ["files", fileId],
    queryFn: () => API.getFile(fileId),
  };
};

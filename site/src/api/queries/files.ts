import * as API from "api/api";

export const uploadFile = () => {
  return {
    mutationFn: API.uploadFile,
  };
};

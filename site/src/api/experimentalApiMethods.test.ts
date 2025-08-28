import { ExperimentalApiMethods } from "./api";
import { AxiosInstance } from "axios";

describe("ExperimentalApiMethods", () => {
  let experimentalApiMethods: ExperimentalApiMethods;
  let mockAxios: jest.Mocked<AxiosInstance>;

  beforeEach(() => {
    mockAxios = {
      get: jest.fn(),
      post: jest.fn(),
    } as unknown as jest.Mocked<AxiosInstance>;
    experimentalApiMethods = new ExperimentalApiMethods(mockAxios);
  });

  describe("getTasks", () => {
    it("excludes prebuild workspaces", async () => {
      const mockWorkspaces = {
        workspaces: [
          { id: "1", is_prebuild: false, latest_build: { id: "build1" } },
          { id: "2", is_prebuild: true, latest_build: { id: "build2" } },
          { id: "3", is_prebuild: false, latest_build: { id: "build3" } },
        ],
      };

      const mockPrompts = {
        prompts: {
          build1: "prompt1",
          build2: "prompt2",
          build3: "prompt3",
        },
      };

      mockAxios.get.mockResolvedValueOnce({ data: mockWorkspaces });
      mockAxios.get.mockResolvedValueOnce({ data: mockPrompts });

      const result = await experimentalApiMethods.getTasks({});

      expect(mockAxios.get).toHaveBeenCalledWith("/api/v2/workspaces", {
        params: { q: "has-ai-task:true is_prebuild:false" },
      });

      expect(result).toHaveLength(2);
      expect(result[0].workspace.id).toBe("1");
      expect(result[1].workspace.id).toBe("3");
    });
  });
});
import { ProvisionerJobLog } from "api/typesGenerated";
import { groupLogsByStage } from "./WorkspaceBuildLogs";

describe("groupLogsByStage", () => {
  it("should group them by stage", () => {
    const input: ProvisionerJobLog[] = [
      {
        id: 1,
        created_at: "oct 13",
        log_source: "provisioner",
        log_level: "debug",
        stage: "build",
        output: "test",
      },
      {
        id: 2,
        created_at: "oct 13",
        log_source: "provisioner",
        log_level: "debug",
        stage: "cleanup",
        output: "test",
      },
      {
        id: 3,
        created_at: "oct 13",
        log_source: "provisioner",
        log_level: "debug",
        stage: "cleanup",
        output: "done",
      },
    ];

    const actual = groupLogsByStage(input);

    expect(actual["cleanup"].length).toBe(2);
  });
});

import { useMachine } from "@xstate/react";
import { FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useParams } from "react-router-dom";
import { pageTitle } from "../../utils/page";
import { workspaceBuildMachine } from "../../xServices/workspaceBuild/workspaceBuildXService";
import { WorkspaceBuildPageView } from "./WorkspaceBuildPageView";
import { useQuery } from "@tanstack/react-query";
import { getWorkspaceBuilds } from "api/api";
import dayjs from "dayjs";

export const WorkspaceBuildPage: FC = () => {
  const params = useParams() as {
    username: string;
    workspace: string;
    buildNumber: string;
  };
  const workspaceName = params.workspace;
  const buildNumber = Number(params.buildNumber);
  const username = params.username.replace("@", "");
  const [buildState, send] = useMachine(workspaceBuildMachine, {
    context: { username, workspaceName, buildNumber, timeCursor: new Date() },
  });
  const { logs, build } = buildState.context;
  const { data: builds } = useQuery({
    queryKey: ["builds", username, build?.workspace_id],
    queryFn: () => {
      return getWorkspaceBuilds(
        build?.workspace_id ?? "",
        dayjs().add(-30, "day").toDate(),
      );
    },
    enabled: Boolean(build),
  });

  useEffect(() => {
    send("RESET", { buildNumber, timeCursor: new Date() });
  }, [buildNumber, send]);

  return (
    <>
      <Helmet>
        <title>
          {build
            ? pageTitle(
                `Build #${build.build_number} · ${build.workspace_name}`,
              )
            : ""}
        </title>
      </Helmet>

      <WorkspaceBuildPageView
        logs={logs}
        build={build}
        builds={builds}
        activeBuildNumber={buildNumber}
      />
    </>
  );
};

export default WorkspaceBuildPage;

import { useMachine } from "@xstate/react";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { editGroupMachine } from "xServices/groups/editGroupXService";
import SettingsGroupPageView from "./SettingsGroupPageView";

export const SettingsGroupPage: FC = () => {
  const { groupId } = useParams();
  if (!groupId) {
    throw new Error("Group ID not defined.");
  }

  const navigate = useNavigate();

  const navigateToGroup = () => {
    navigate(`/groups/${groupId}`);
  };

  const [editState, sendEditEvent] = useMachine(editGroupMachine, {
    context: {
      groupId,
    },
    actions: {
      onUpdate: navigateToGroup,
    },
  });
  const { error, group } = editState.context;

  return (
    <>
      <Helmet>
        <title>{pageTitle("Settings Group")}</title>
      </Helmet>

      <SettingsGroupPageView
        onCancel={navigateToGroup}
        onSubmit={(data) => {
          sendEditEvent({ type: "UPDATE", data });
        }}
        group={group}
        formErrors={error}
        isLoading={editState.matches("loading")}
        isUpdating={editState.matches("updating")}
      />
    </>
  );
};
export default SettingsGroupPage;

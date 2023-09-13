import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import { useMachine } from "@xstate/react";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import { useFeatureVisibility } from "hooks/useFeatureVisibility";
import { useOrganizationId } from "hooks/useOrganizationId";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { templateACLMachine } from "xServices/template/templateACLXService";
import { useTemplateSettingsContext } from "../TemplateSettingsLayout";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";
import { docs } from "utils/docs";

export const TemplatePermissionsPage: FC<
  React.PropsWithChildren<unknown>
> = () => {
  const organizationId = useOrganizationId();
  const { template, permissions } = useTemplateSettingsContext();
  const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
  const [state, send] = useMachine(templateACLMachine, {
    context: { templateId: template.id },
  });
  const { templateACL, userToBeUpdated, groupToBeUpdated } = state.context;

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "Permissions"])}</title>
      </Helmet>
      <ChooseOne>
        <Cond condition={!isTemplateRBACEnabled}>
          <Paywall
            message="Template permissions"
            description="Manage your template permissions to allow users or groups to view or admin the template. To use this feature, you have to upgrade your account."
            cta={
              <Stack direction="row" alignItems="center">
                <Link
                  href={docs("/admin/upgrade")}
                  target="_blank"
                  rel="noreferrer"
                >
                  <Button
                    startIcon={<ArrowRightAltOutlined />}
                    variant="contained"
                  >
                    See how to upgrade
                  </Button>
                </Link>
                <Link
                  href={docs("/admin/rbac")}
                  target="_blank"
                  rel="noreferrer"
                >
                  Read the docs
                </Link>
              </Stack>
            }
          />
        </Cond>
        <Cond>
          <TemplatePermissionsPageView
            organizationId={organizationId}
            templateID={template.id}
            templateACL={templateACL}
            canUpdatePermissions={Boolean(permissions?.canUpdateTemplate)}
            onAddUser={(user, role, reset) => {
              send("ADD_USER", { user, role, onDone: reset });
            }}
            isAddingUser={state.matches("addingUser")}
            onUpdateUser={(user, role) => {
              send("UPDATE_USER_ROLE", { user, role });
            }}
            updatingUser={userToBeUpdated}
            onRemoveUser={(user) => {
              send("REMOVE_USER", { user });
            }}
            onAddGroup={(group, role, reset) => {
              send("ADD_GROUP", { group, role, onDone: reset });
            }}
            isAddingGroup={state.matches("addingGroup")}
            onUpdateGroup={(group, role) => {
              send("UPDATE_GROUP_ROLE", { group, role });
            }}
            updatingGroup={groupToBeUpdated}
            onRemoveGroup={(group) => {
              send("REMOVE_GROUP", { group });
            }}
          />
        </Cond>
      </ChooseOne>
    </>
  );
};

export default TemplatePermissionsPage;

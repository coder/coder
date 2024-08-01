import type { Interpolation, Theme } from "@emotion/react";
import AddOutlined from "@mui/icons-material/AddOutlined";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import { LoadingButton } from "@mui/lab";
import { createFilterOptions } from "@mui/material/Autocomplete";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { useState, type FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import type { Role } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
  TableLoader,
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { permissionsToCheck } from "contexts/auth/permissions";
import { useClickableTableRow } from "hooks";
import { docs } from "utils/docs";

export type CustomRolesPageViewProps = {
  roles: Role[] | undefined;
  canAssignOrgRole: boolean;
  isCustomRolesEnabled: boolean;
};

// const filter = createFilterOptions<Role>();

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
  roles,
  canAssignOrgRole,
  isCustomRolesEnabled,
}) => {
  const isLoading = Boolean(roles === undefined);
  const isEmpty = Boolean(roles && roles.length === 0);
  // const [selectedRole, setSelectedRole] = useState<Role | null>(null);
  console.log({ roles });
  return (
    <>
      <ChooseOne>
        <Cond condition={!isCustomRolesEnabled}>
          <Paywall
            message="Custom Roles"
            description="Organize users into groups with restricted access to templates. You need an Enterprise license to use this feature."
            documentationLink={docs("/admin/groups")}
          />
        </Cond>
        <Cond>
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="33%">Name</TableCell>
                  <TableCell width="33%">Display Name</TableCell>
                  <TableCell width="33%">Permissions</TableCell>
                  <TableCell width="1%"></TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                <ChooseOne>
                  <Cond condition={isLoading}>
                    <TableLoader />
                  </Cond>

                  <Cond condition={isEmpty}>
                    <TableRow>
                      <TableCell colSpan={999}>
                        <EmptyState
                          message="No groups yet"
                          description={
                            canAssignOrgRole
                              ? "Create your first custom role"
                              : "You don't have permission to create a custom role"
                          }
                          cta={
                            canAssignOrgRole && (
                              <Button
                                component={RouterLink}
                                to="create"
                                startIcon={<AddOutlined />}
                                variant="contained"
                              >
                                Create custom role
                              </Button>
                            )
                          }
                        />
                      </TableCell>
                    </TableRow>
                  </Cond>

                  <Cond>
                    {roles?.map((role) => (
                      <RoleRow key={role.name} role={role} />
                    ))}
                  </Cond>
                </ChooseOne>
              </TableBody>
            </Table>
          </TableContainer>
        </Cond>
      </ChooseOne>
    </>
  );
};

interface RoleRowProps {
  role: Role;
}

const RoleRow: FC<RoleRowProps> = ({ role }) => {
  const navigate = useNavigate();
  const rowProps = useClickableTableRow({
    onClick: () => navigate(role.name),
  });

  return (
    <TableRow data-testid={`role-${role.name}`} {...rowProps}>
      <TableCell>{role.name}</TableCell>

      <TableCell css={styles.secondary}>{role.display_name}</TableCell>

      <TableCell css={styles.secondary}>
        {role.organization_permissions.length}
      </TableCell>

      <TableCell>
        <div css={styles.arrowCell}>
          <KeyboardArrowRight css={styles.arrowRight} />
        </div>
      </TableCell>
    </TableRow>
  );
};

const styles = {
  arrowRight: (theme) => ({
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  }),
  arrowCell: {
    display: "flex",
  },
  secondary: (theme) => ({
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export default CustomRolesPageView;

import { css } from "@emotion/css";
import type { Interpolation, Theme } from "@emotion/react";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import PersonAdd from "@mui/icons-material/PersonAdd";
import { LoadingButton } from "@mui/lab";
import { Table, TableBody, TableContainer, TextField } from "@mui/material";
import Autocomplete, { createFilterOptions } from "@mui/material/Autocomplete";
import AvatarGroup from "@mui/material/AvatarGroup";
import Skeleton from "@mui/material/Skeleton";
import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { useState, type FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { RBACResourceActions } from "api/rbacresources_gen";
import type { Group, Role } from "api/typesGenerated";
import { AvatarData } from "components/AvatarData/AvatarData";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { GroupAvatar } from "components/GroupAvatar/GroupAvatar";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
  TableLoader,
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { permissionsToCheck } from "contexts/auth/permissions";
import { useClickableTableRow } from "hooks";
import { docs } from "utils/docs";

export type CustomRolesPageViewProps = {
  roles: Role[] | undefined;
  canCreateGroup: boolean;
  isTemplateRBACEnabled: boolean;
};

const filter = createFilterOptions<Role>();

export const CustomRolesPageView: FC<CustomRolesPageViewProps> = ({
  roles,
  canCreateGroup,
  isTemplateRBACEnabled,
}) => {
  const isLoading = Boolean(roles === undefined);
  const isEmpty = Boolean(roles && roles.length === 0);
  const [selectedRole, setSelectedRole] = useState<Role | null>(null);
  console.log({ selectedRole });

  return (
    <>
      <ChooseOne>
        <Cond condition={!isTemplateRBACEnabled}>
          <Paywall
            message="Custom Roles"
            description="Organize users into groups with restricted access to templates. You need an Enterprise license to use this feature."
            documentationLink={docs("/admin/groups")}
          />
        </Cond>
        <Cond>
          <Stack
            direction="row"
            alignItems="center"
            spacing={1}
            css={styles.rolesDropdown}
          >
            <Autocomplete
              value={selectedRole}
              onChange={(_, newValue) => {
                console.log("onChange: ", newValue);
                if (typeof newValue === "string") {
                  console.log("0");
                  setSelectedRole({
                    name: newValue,
                    display_name: newValue,
                    site_permissions: [],
                    organization_permissions: [],
                    user_permissions: [],
                  });
                } else if (newValue && newValue.display_name) {
                  console.log("1");
                  // Create a new value from the user input
                  // setSelectedRole({ ...newValue, display_name: newValue.name });
                  setSelectedRole(newValue);
                } else {
                  console.log("2");
                  setSelectedRole(newValue);
                }
              }}
              isOptionEqualToValue={(option: Role, value: Role) =>
                option.name === value.name
              }
              filterOptions={(options, params) => {
                const filtered = filter(options, params);

                const { inputValue } = params;
                // Suggest the creation of a new value
                const isExisting = options.some(
                  (option) => inputValue === option.display_name,
                );
                if (inputValue !== "" && !isExisting) {
                  filtered.push({
                    name: inputValue,
                    display_name: `Add ${inputValue}`,
                    site_permissions: [],
                    organization_permissions: [],
                    user_permissions: [],
                  });
                }

                return filtered;
              }}
              selectOnFocus
              clearOnBlur
              handleHomeEndKeys
              id="custom-role"
              options={roles || []}
              getOptionLabel={(option) => {
                // console.log("getOptionLabel: ", option);
                // Value selected with enter, right from the input
                if (typeof option === "string") {
                  return option;
                }
                // Add "xxx" option created dynamically
                if (option.name) {
                  return option.name;
                }
                // Regular option
                return option.display_name;
              }}
              renderOption={(props, option) => {
                const { key, ...optionProps } = props;
                return (
                  <li key={key} {...optionProps}>
                    {option.display_name}
                  </li>
                );
              }}
              sx={{ width: 300 }}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label="Display Name"
                  InputLabelProps={{
                    shrink: true,
                  }}
                />
              )}
            />

            <LoadingButton
              loadingPosition="start"
              // disabled={!selectedUser}
              type="submit"
              startIcon={<PersonAdd />}
              loading={isLoading}
            >
              Save Custom Role
            </LoadingButton>
          </Stack>

          <TableContainer>
            <Table>
              <TableBody>
                <ChooseOne>
                  <Cond condition={isLoading}>
                    <TableLoader />
                  </Cond>

                  <Cond condition={isEmpty}>
                    <TableRow>
                      <TableCell colSpan={999}>
                        <EmptyState
                          message="No custom roles yet"
                          description={
                            canCreateGroup
                              ? "Create your first custom role"
                              : "You don't have permission to create a custom role"
                          }
                        />
                      </TableCell>
                    </TableRow>
                  </Cond>

                  <Cond>
                    {Object.entries(RBACResourceActions).map(([key, value]) => {
                      return (
                        <TableRow key={key}>
                          <TableCell>
                            <li key={key} css={styles.checkBoxes}>
                              <input type="checkbox" /> {key}
                              <ul css={styles.checkBoxes}>
                                {Object.entries(value).map(([key, value]) => {
                                  return (
                                    <li key={key}>
                                      <span css={styles.actionText}>
                                        <input type="checkbox" /> {key}
                                      </span>{" "}
                                      -{" "}
                                      <span css={styles.actionDescription}>
                                        {value}
                                      </span>
                                    </li>
                                  );
                                })}
                              </ul>
                            </li>
                          </TableCell>
                        </TableRow>
                      );
                    })}
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

const styles = {
  rolesDropdown: {
    marginBottom: 20,
  },
  checkBoxes: {
    margin: 0,
    listStyleType: "none",
  },
  actionText: (theme) => ({
    color: theme.palette.text.primary,
  }),
  actionDescription: (theme) => ({
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export default CustomRolesPageView;

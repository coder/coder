import { type Interpolation, type Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import AddOutlined from "@mui/icons-material/AddOutlined";
import KeyboardArrowRight from "@mui/icons-material/KeyboardArrowRight";
import AvatarGroup from "@mui/material/AvatarGroup";
import { AvatarData } from "components/AvatarData/AvatarData";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Stack } from "components/Stack/Stack";
import {
  TableLoaderSkeleton,
  TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { Paywall } from "components/Paywall/Paywall";
import type { Group } from "api/typesGenerated";
import { GroupAvatar } from "components/GroupAvatar/GroupAvatar";
import { docs } from "utils/docs";
import Skeleton from "@mui/material/Skeleton";
import { AvatarDataSkeleton } from "components/AvatarData/AvatarDataSkeleton";

export type GroupsPageViewProps = {
  groups: Group[] | undefined;
  canCreateGroup: boolean;
  isTemplateRBACEnabled: boolean;
};

export const GroupsPageView: FC<GroupsPageViewProps> = ({
  groups,
  canCreateGroup,
  isTemplateRBACEnabled,
}) => {
  const isLoading = Boolean(groups === undefined);
  const isEmpty = Boolean(groups && groups.length === 0);
  const navigate = useNavigate();

  return (
    <>
      <ChooseOne>
        <Cond condition={!isTemplateRBACEnabled}>
          <Paywall
            message="Groups"
            description="Organize users into groups with restricted access to templates. You need an Enterprise license to use this feature."
            cta={
              <Stack direction="row" alignItems="center">
                <Button
                  href={docs("/enterprise")}
                  target="_blank"
                  rel="noreferrer"
                  startIcon={<ArrowRightAltOutlined />}
                  variant="contained"
                  color="primary"
                >
                  Learn about Enterprise
                </Button>

                <Link
                  href={docs("/admin/groups")}
                  target="_blank"
                  rel="noreferrer"
                >
                  Read the documentation
                </Link>
              </Stack>
            }
          />
        </Cond>
        <Cond>
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell width="50%">Name</TableCell>
                  <TableCell width="49%">Users</TableCell>
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
                            canCreateGroup
                              ? "Create your first group"
                              : "You don't have permission to create a group"
                          }
                          cta={
                            canCreateGroup && (
                              <Button
                                component={RouterLink}
                                to="/groups/create"
                                startIcon={<AddOutlined />}
                                variant="contained"
                              >
                                Create group
                              </Button>
                            )
                          }
                        />
                      </TableCell>
                    </TableRow>
                  </Cond>

                  <Cond>
                    {groups?.map((group) => {
                      const groupPageLink = `/groups/${group.id}`;

                      return (
                        <TableRow
                          hover
                          key={group.id}
                          data-testid={`group-${group.id}`}
                          tabIndex={0}
                          onClick={() => {
                            navigate(groupPageLink);
                          }}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") {
                              navigate(groupPageLink);
                            }
                          }}
                          css={styles.clickableTableRow}
                        >
                          <TableCell>
                            <AvatarData
                              avatar={
                                <GroupAvatar
                                  name={group.display_name || group.name}
                                  avatarURL={group.avatar_url}
                                />
                              }
                              title={group.display_name || group.name}
                              subtitle={`${group.members.length} members`}
                            />
                          </TableCell>

                          <TableCell>
                            {group.members.length === 0 && "-"}
                            <AvatarGroup
                              max={10}
                              total={group.members.length}
                              css={{ justifyContent: "flex-end" }}
                            >
                              {group.members.map((member) => (
                                <UserAvatar
                                  key={member.username}
                                  username={member.username}
                                  avatarURL={member.avatar_url}
                                />
                              ))}
                            </AvatarGroup>
                          </TableCell>

                          <TableCell>
                            <div css={styles.arrowCell}>
                              <KeyboardArrowRight css={styles.arrowRight} />
                            </div>
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

const TableLoader = () => {
  return (
    <TableLoaderSkeleton>
      <TableRowSkeleton>
        <TableCell>
          <div css={{ display: "flex", alignItems: "center", gap: 8 }}>
            <AvatarDataSkeleton />
          </div>
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
        <TableCell>
          <Skeleton variant="text" width="25%" />
        </TableCell>
      </TableRowSkeleton>
    </TableLoaderSkeleton>
  );
};

const styles = {
  clickableTableRow: (theme) => ({
    cursor: "pointer",

    "&:hover td": {
      backgroundColor: theme.palette.action.hover,
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.primary.main}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: `16px !important`,
    },
  }),
  arrowRight: (theme) => ({
    color: theme.palette.text.secondary,
    width: 20,
    height: 20,
  }),
  arrowCell: {
    display: "flex",
  },
} satisfies Record<string, Interpolation<Theme>>;

export default GroupsPageView;

import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import { type ChangeEvent, type FC, useState } from "react";
import { css } from "@emotion/react";
import type { Group, User } from "api/typesGenerated";
import { AvatarData } from "components/AvatarData/AvatarData";
import { getGroupSubtitle } from "utils/groups";
import { useDebouncedFunction } from "hooks/debounce";
import { useQuery } from "react-query";
import { templaceACLAvailable } from "api/queries/templates";
import { prepareQuery } from "utils/filters";

export type UserOrGroupAutocompleteValue = User | Group | null;

export type UserOrGroupAutocompleteProps = {
  value: UserOrGroupAutocompleteValue;
  onChange: (value: UserOrGroupAutocompleteValue) => void;
  templateID: string;
  exclude: UserOrGroupAutocompleteValue[];
};

export const UserOrGroupAutocomplete: FC<UserOrGroupAutocompleteProps> = ({
  value,
  onChange,
  templateID,
  exclude,
}) => {
  const [autoComplete, setAutoComplete] = useState({
    value: "",
    open: false,
  });
  const aclAvailableQuery = useQuery({
    ...templaceACLAvailable(templateID, {
      q: prepareQuery(encodeURI(autoComplete.value)),
      limit: 25,
    }),
    enabled: autoComplete.open,
    keepPreviousData: true,
  });
  const options = aclAvailableQuery.data
    ? [
        ...aclAvailableQuery.data.groups,
        ...aclAvailableQuery.data.users,
      ].filter((result) => {
        const excludeIds = exclude.map(
          (optionToExclude) => optionToExclude?.id,
        );
        return !excludeIds.includes(result.id);
      })
    : [];

  const { debounced: handleFilterChange } = useDebouncedFunction(
    (event: ChangeEvent<HTMLInputElement>) => {
      setAutoComplete((state) => ({
        ...state,
        value: event.target.value,
      }));
    },
    500,
  );

  return (
    <Autocomplete
      noOptionsText="No users or groups found"
      value={value}
      id="user-or-group-autocomplete"
      open={autoComplete.open}
      onOpen={() => {
        setAutoComplete((state) => ({
          ...state,
          open: true,
        }));
      }}
      onClose={() => {
        setAutoComplete({
          value: isGroup(value) ? value.display_name : value?.email ?? "",
          open: false,
        });
      }}
      onChange={(_, newValue) => {
        onChange(newValue);
      }}
      isOptionEqualToValue={(option, value) => option.id === value.id}
      getOptionLabel={(option) =>
        isGroup(option) ? option.display_name || option.name : option.email
      }
      renderOption={(props, option) => {
        const isOptionGroup = isGroup(option);

        return (
          <li {...props}>
            <AvatarData
              title={
                isOptionGroup
                  ? option.display_name || option.name
                  : option.username
              }
              subtitle={isOptionGroup ? getGroupSubtitle(option) : option.email}
              src={option.avatar_url}
            />
          </li>
        );
      }}
      options={options}
      loading={aclAvailableQuery.isFetching}
      css={autoCompleteStyles}
      renderInput={(params) => (
        <>
          <TextField
            {...params}
            margin="none"
            size="small"
            placeholder="Search for user or group"
            InputProps={{
              ...params.InputProps,
              onChange: handleFilterChange,
              endAdornment: (
                <>
                  {aclAvailableQuery.isFetching ? (
                    <CircularProgress size={16} />
                  ) : null}
                  {params.InputProps.endAdornment}
                </>
              ),
            }}
          />
        </>
      )}
    />
  );
};

const isGroup = (value: UserOrGroupAutocompleteValue): value is Group => {
  return value !== null && "members" in value;
};

const autoCompleteStyles = css`
  width: 300px;

  & .MuiFormControl-root {
    width: 100%;
  }

  & .MuiInputBase-root {
    width: 100%;
  }
`;

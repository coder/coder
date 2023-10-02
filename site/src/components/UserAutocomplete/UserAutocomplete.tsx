import CircularProgress from "@mui/material/CircularProgress";
import { makeStyles } from "@mui/styles";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import { ChangeEvent, ComponentProps, FC, useState } from "react";
import Box from "@mui/material/Box";
import { useDebouncedFunction } from "hooks/debounce";
import { useQuery } from "@tanstack/react-query";
import { users } from "api/queries/users";
import { prepareQuery } from "utils/filters";

export type UserAutocompleteProps = {
  value: User | null;
  onChange: (user: User | null) => void;
  label?: string;
  className?: string;
  size?: ComponentProps<typeof TextField>["size"];
};

export const UserAutocomplete: FC<UserAutocompleteProps> = ({
  value,
  onChange,
  label,
  className,
  size = "small",
}) => {
  const styles = useStyles();
  const [autoComplete, setAutoComplete] = useState<{
    value: string;
    open: boolean;
  }>({
    value: value?.email ?? "",
    open: false,
  });
  const usersQuery = useQuery({
    ...users({
      q: prepareQuery(encodeURI(autoComplete.value)),
      limit: 25,
    }),
    enabled: autoComplete.open,
    keepPreviousData: true,
  });

  const { debounced: debouncedInputOnChange } = useDebouncedFunction(
    (event: ChangeEvent<HTMLInputElement>) => {
      setAutoComplete((state) => ({
        ...state,
        value: event.target.value,
      }));
    },
    750,
  );

  return (
    <Autocomplete
      // Since the values are filtered by the API we don't need to filter them
      // in the FE because it can causes some mismatches.
      filterOptions={(user) => user}
      noOptionsText="No users found"
      className={className}
      options={usersQuery.data?.users ?? []}
      loading={usersQuery.isLoading}
      value={value}
      id="user-autocomplete"
      open={autoComplete.open}
      onOpen={() => {
        setAutoComplete((state) => ({
          ...state,
          open: true,
        }));
      }}
      onClose={() => {
        setAutoComplete({
          value: value?.email ?? "",
          open: false,
        });
      }}
      onChange={(_, newValue) => {
        onChange(newValue);
      }}
      isOptionEqualToValue={(option: User, value: User) =>
        option.username === value.username
      }
      getOptionLabel={(option) => option.email}
      renderOption={(props, option) => (
        <Box component="li" {...props}>
          <AvatarData
            title={option.username}
            subtitle={option.email}
            src={option.avatar_url}
          />
        </Box>
      )}
      renderInput={(params) => (
        <TextField
          {...params}
          fullWidth
          size={size}
          label={label}
          placeholder="User email or username"
          className={styles.textField}
          InputProps={{
            ...params.InputProps,
            onChange: debouncedInputOnChange,
            startAdornment: value && (
              <Avatar size="sm" src={value.avatar_url}>
                {value.username}
              </Avatar>
            ),
            endAdornment: (
              <>
                {usersQuery.isFetching && autoComplete.open ? (
                  <CircularProgress size={16} />
                ) : null}
                {params.InputProps.endAdornment}
              </>
            ),
            classes: {
              root: styles.inputRoot,
            },
          }}
          InputLabelProps={{
            shrink: true,
          }}
        />
      )}
    />
  );
};

export const useStyles = makeStyles((theme) => ({
  textField: {
    "&:not(:has(label))": {
      margin: 0,
    },
  },
  inputRoot: {
    paddingLeft: `${theme.spacing(1.75)} !important`, // Same padding left as input
    gap: theme.spacing(0.5),
  },
}));

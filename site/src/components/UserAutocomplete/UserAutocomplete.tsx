import CircularProgress from "@mui/material/CircularProgress";
import { makeStyles } from "@mui/styles";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import { useMachine } from "@xstate/react";
import { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import debounce from "just-debounce-it";
import { ChangeEvent, ComponentProps, FC, useEffect, useState } from "react";
import { searchUserMachine } from "xServices/users/searchUserXService";
import { useTranslation } from "react-i18next";
import Box from "@mui/material/Box";

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
  const { t } = useTranslation("common");
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
  const [searchState, sendSearch] = useMachine(searchUserMachine);
  const { searchResults } = searchState.context;

  // seed list of options on the first page load if a user pases in a value
  // since some organizations have long lists of users, we do not load all options on page load.
  useEffect(() => {
    if (value) {
      sendSearch("SEARCH", { query: value.email });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- TODO look into this
  }, []);

  const handleFilterChange = debounce(
    (event: ChangeEvent<HTMLInputElement>) => {
      sendSearch("SEARCH", { query: event.target.value });
    },
    1000,
  );

  return (
    <Autocomplete
      noOptionsText={t("forms.typeToSearch")}
      className={className}
      options={searchResults ?? []}
      loading={searchState.matches("searching")}
      value={value}
      id="user-autocomplete"
      open={isAutocompleteOpen}
      onOpen={() => {
        setIsAutocompleteOpen(true);
      }}
      onClose={() => {
        setIsAutocompleteOpen(false);
      }}
      onChange={(_, newValue) => {
        if (newValue === null) {
          sendSearch("CLEAR_RESULTS");
        }

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
        <>
          {/* eslint-disable-next-line @typescript-eslint/ban-ts-comment -- Need it */}
          {/* @ts-ignore -- Issue from lib https://github.com/i18next/react-i18next/issues/1543 */}
          <TextField
            {...params}
            fullWidth
            size={size}
            label={label}
            placeholder="User email or username"
            className={styles.textField}
            InputProps={{
              ...params.InputProps,
              onChange: handleFilterChange,
              startAdornment: value && (
                <Avatar size="sm" src={value.avatar_url}>
                  {value.username}
                </Avatar>
              ),
              endAdornment: (
                <>
                  {searchState.matches("searching") ? (
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
        </>
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

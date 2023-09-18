import CircularProgress from "@mui/material/CircularProgress";
import { makeStyles } from "@mui/styles";
import TextField from "@mui/material/TextField";
import Autocomplete from "@mui/material/Autocomplete";
import { useMachine } from "@xstate/react";
import { User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/AvatarData/AvatarData";
import {
  ChangeEvent,
  ComponentProps,
  FC,
  useEffect,
  useRef,
  useState,
} from "react";
import { searchUserMachine } from "xServices/users/searchUserXService";
import Box from "@mui/material/Box";
import { useDebouncedFunction } from "hooks/debounce";

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
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false);
  const [searchState, sendSearch] = useMachine(searchUserMachine);
  const { searchResults } = searchState.context;

  // Seed list of options on the first page load if a user passes in a value.
  // Since some organizations have long lists of users, we do not want to load
  // all options on page load.
  const onMountRef = useRef(value);
  useEffect(() => {
    const mountValue = onMountRef.current;
    if (mountValue) {
      sendSearch("SEARCH", { query: mountValue.email });
    }

    // This isn't in XState's docs, but its source code guarantees that the
    // memory reference of sendSearch will stay stable across renders. This
    // useEffect call will behave like an on-mount effect and will not ever need
    // to resynchronize
  }, [sendSearch]);

  const { debounced: debouncedOnChange } = useDebouncedFunction(
    (event: ChangeEvent<HTMLInputElement>) => {
      sendSearch("SEARCH", { query: event.target.value });
    },
    1000,
  );

  return (
    <Autocomplete
      noOptionsText="Start typing to search..."
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
          <TextField
            {...params}
            fullWidth
            size={size}
            label={label}
            placeholder="User email or username"
            className={styles.textField}
            InputProps={{
              ...params.InputProps,
              onChange: debouncedOnChange,
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
